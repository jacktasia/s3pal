package main

import (
	"encoding/base64"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strconv"
	"strings"
	"time"
)

func StartServer(config S3palConfig) {

	if config.Server.Debug {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.Default()

	listCache := ListCache{}

	listCache.timeout = map[string]int64{}
	listCache.items = map[string][]string{}

	r.Use(CORSMiddleware())

	faviconStr := "R0lGODlhAQABAIAAAAUEBAAAACwAAAAAAQABAAACAkQBADs="
	favicon, _ := base64.StdEncoding.DecodeString(faviconStr)
	faviconLen := strconv.Itoa(len(favicon))

	r.GET("/favicon.ico", func(g *gin.Context) {
		g.Writer.Header().Set("Content-Type", "image/gif")
		g.Writer.Header().Set("Content-Length", faviconLen)
		g.Writer.Write(favicon)
	})

	r.OPTIONS("/upload/file", func(g *gin.Context) {
		g.String(204, "")
	})

	r.GET("/", func(g *gin.Context) {
		content := getUploadForm(config)
		g.Writer.Header().Set("Content-Type", "text/html")
		g.Writer.Header().Set("Content-Length", strconv.Itoa(len(content)))
		g.Writer.Write([]byte(content))
	})

	r.POST("/upload/url", func(c *gin.Context) {
		url := c.Request.FormValue("url")
		prefix := c.Request.FormValue("prefix")

		uploaded := false

		var newFilename string
		var err error
		if strings.HasPrefix(url, "http") {
			newFilename, err = UploadPathOrURL(config.Aws, url, prefix)
			if err == nil {
				uploaded = true
			}
		}

		if config.Server.CacheEnabled && config.Server.CacheBustOnUpload {
			log.Println("Cache BUST (upload url)")
			listCache.timeout[prefix] = 0
		}

		if uploaded {
			response := map[string]string{
				"status":   "ok",
				"filename": newFilename,
			}
			c.JSON(200, response)
		} else {
			response := map[string]string{
				"status": "error",
				"reason": "error uploading",
			}
			c.JSON(500, response)
		}
	})

	r.POST("/upload/file", func(c *gin.Context) {
		file, header, err := c.Request.FormFile("file")

		if err != nil {
			return
		}

		prefix := c.Request.FormValue("prefix")

		// create a temp file
		out, err := ioutil.TempFile("/tmp", "uploaded_")
		if err != nil {
			log.Println("ERROR TEMP FILE")
			return
		}

		// write the content from POST to the file
		_, err = io.Copy(out, file)
		if err != nil {
			log.Println("ERROR COPYING")
		}

		file.Close()

		fi, _ := out.Stat()

		newFilename := makeFilename(prefix)
		path := out.Name()
		out.Close()
		uploaded := false

		max := config.Server.MaxPostBytes

		// handle max post byte
		// negative max is any size
		tooBig := false
		if max == 0 {
			max = 4000000
		}

		if max > 0 {
			tooBig = fi.Size() > max
		}

		if !tooBig {
			err := uploadToS3(config.Aws, path, header.Header.Get("Content-Type"), newFilename)

			if err == nil {
				uploaded = true
			} else {
				log.Println(err)
			}
		}

		// done with uploaded file
		_ = os.Remove(path)

		if config.Server.CacheEnabled && config.Server.CacheBustOnUpload {
			log.Println("Cache BUST (upload file)")
			listCache.timeout[prefix] = 0
		}

		// respond
		if tooBig {
			response := map[string]string{
				"status": "error",
				"reason": fmt.Sprintf("Upload too big. %v > %v", fi.Size(), max),
			}
			c.JSON(400, response)
		} else if uploaded {
			response := map[string]string{
				"status":   "ok",
				"filename": newFilename,
				"url":      makeUrl(config.Aws, newFilename),
			}
			c.JSON(200, response)
		} else {
			response := map[string]string{
				"status": "error",
				"reason": "error uploading",
			}
			c.JSON(500, response)
		}
	})

	r.GET("/list", func(c *gin.Context) {
		prefix := c.Request.FormValue("prefix")

		makeRequest := true

		if config.Server.CacheEnabled {
			now := time.Now().Unix()
			makeRequest = now > listCache.timeout[prefix]
		}

		var items []string
		var err error
		if makeRequest {
			items, err = listS3Bucket(config.Aws, prefix)

			if config.Server.CacheEnabled && err == nil {
				log.Println("Cache MISS")
				listCache.items[prefix] = make([]string, len(items))
				copy(listCache.items[prefix], items)
				listCache.timeout[prefix] = time.Now().Unix() + config.Server.CacheTTL
			}
		} else {
			items = make([]string, len(listCache.items[prefix]))
			copy(items, listCache.items[prefix])
			log.Println("Cache HIT")
		}

		if err == nil {
			c.JSON(200, items)
		} else {
			response := map[string]string{
				"status": "error",
				"reason": "error listing",
			}
			c.JSON(500, response)
		}
	})

	port := forcePort(config.Server.Port)
	if port != config.Server.Port && config.Server.NoForcePort {
		fmt.Printf("\nNot Running! Port %v already in use.\n\n", config.Server.Port)
		fmt.Printf("'no_force_port' option is enabled.\n\n")

		return
	}

	fmt.Printf("\ns3pal is running on port %v...\n\n", port)
	r.Run(fmt.Sprintf(":%d", port))
}
