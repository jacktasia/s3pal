package main

import (
	"encoding/base64"
	"fmt"
	"github.com/gin-gonic/gin"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

func forcePort(port int) int {
	tryPort := ":" + strconv.Itoa(port)
	if port > 65535 {
		panic("Invalid Server Port " + tryPort)
	}

	l, err := net.Listen("tcp", tryPort)
	if err != nil {
		return forcePort(port + 1)
	}

	l.Close()
	return port
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")

		if c.Request.Method == "OPTIONS" {
			//c.String(204, "")
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}

func (s *S3pal) startServer() {

	if s.Config.Server.Debug {
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

	if len(s.Config.Server.StaticPath) > 0 {
		path := s.Config.Server.StaticPath
		fileInfo, err := os.Stat(path)
		setupStatic := true
		if err != nil {
			fmt.Printf("\nServer static path ignored! Could not find folder '%v'\n\n", path)
			setupStatic = false
		}

		if setupStatic && !fileInfo.IsDir() {
			fmt.Printf("\nServer static path ignored! '%v' is NOT a folder.\n\n", path)
			setupStatic = false
		}

		if setupStatic {
			fmt.Printf("\nServing directory '%v' from /static\n", path)
			r.Static("/static", path)
		}
	}

	r.GET("/", func(g *gin.Context) {
		content := s.getUploadForm()
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
			newFilename, err = s.uploadPathOrURL(url, prefix)
			if err == nil {
				uploaded = true
			}
		}

		if s.Config.Server.CacheEnabled && s.Config.Server.CacheBustOnUpload {
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
			fmt.Printf("ERROR: %v (probably no \"file\" field uploaded)\n", err)

			response := map[string]string{
				"status": "error",
				"reason": "No \"file\" field defined",
			}
			c.JSON(400, response)

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

		newFilename := s.makeFilename(prefix, header.Filename)
		path := out.Name()
		out.Close()
		uploaded := false

		max := s.Config.Server.MaxPostBytes

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
			err := s.uploadToS3(path, header.Header.Get("Content-Type"), newFilename)

			if err == nil {
				uploaded = true
			} else {
				log.Println(err)
			}
		}

		// done with uploaded file
		_ = os.Remove(path)

		if s.Config.Server.CacheEnabled && s.Config.Server.CacheBustOnUpload {
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
				"url":      s.makeUrl(newFilename),
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
		urls := strToBool(c.Request.FormValue("urls"))

		makeRequest := true

		if s.Config.Server.CacheEnabled {
			now := time.Now().Unix()
			makeRequest = now > listCache.timeout[prefix]
		}

		var items []string
		var err error
		if makeRequest {
			items, err = s.listS3Bucket(prefix, urls, s.Config.Server.SignURL, s.Config.Server.SignTTL)

			if s.Config.Server.CacheEnabled && err == nil {
				log.Println("Cache MISS")
				listCache.items[prefix] = make([]string, len(items))
				copy(listCache.items[prefix], items)
				listCache.timeout[prefix] = time.Now().Unix() + s.Config.Server.CacheTTL
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

	port := s.Config.Server.Port
	fmt.Printf("\ns3pal is running on port %v...\n\n", port)
	r.Run(fmt.Sprintf(":%d", port))
}
