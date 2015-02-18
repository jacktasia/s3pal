package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/gen/s3"
	"github.com/gin-gonic/gin"
	"gopkg.in/alecthomas/kingpin.v1"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

type tomlConfig struct {
	Aws    AwsConfig
	Server ServerConfig
}

type ServerConfig struct {
	Port              int   `toml:"port"`
	MaxPostBytes      int64 `toml:"max_post_bytes"`
	CacheEnabled      bool  `toml:"cache_enabled"`
	CacheBustOnUpload bool  `toml:"cache_bust_on_upload"`
	CacheTTL          int64 `toml:"cache_ttl"`
}

type AwsConfig struct {
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
	Bucket    string
	Region    string
}

type ListCache struct {
	items   []string
	timeout int64
}

func downloadURL(url string) (*os.File, error) {
	tmp, err := ioutil.TempFile("/tmp", "downloaded_")
	if err != nil {
		return nil, err
	}
	defer tmp.Close()

	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return nil, err
	}

	return tmp, nil
}

func uploadPathOrURL(config AwsConfig, path string, prefix string) (string, error) {
	var toUpload *os.File
	var err error

	if _, err := os.Stat(path); err == nil {
		toUpload, err = os.Open(path)
	} else {
		toUpload, err = downloadURL(path)
	}

	if err != nil {
		return "", err
	}

	bytes, err := ioutil.ReadFile(toUpload.Name())
	contentType := http.DetectContentType(bytes)
	newFilename := makeFilename(prefix)

	err = uploadToS3(config, toUpload.Name(), contentType, newFilename)

	return newFilename, err
}

func makeFilename(prefix string) string {
	return fmt.Sprintf("%suploaded/%s", prefix, uuid.NewUUID().String())
}

func uploadToS3(config AwsConfig, path string, contentType string, filename string) (err error) {
	fd, err := os.Open(path)
	if err != nil {
		log.Printf("Error opening temp: %v", err)
		return err
	}

	defer fd.Close()

	fi, err := fd.Stat()
	if err != nil {
		log.Printf("Error: no input file found in '%s'\n", os.Args[1])
		return err
	}

	bucket := config.Bucket
	if len(contentType) == 0 {
		contentType = "binary/octet-stream"
	}
	creds := aws.Creds(config.AccessKey, config.SecretKey, "")
	cli := s3.New(creds, config.Region, nil)

	objectreq := &s3.PutObjectRequest{
		ACL:           aws.String("public-read"),
		Bucket:        aws.String(bucket),
		Body:          fd,
		ContentLength: aws.Long(int64(fi.Size())),
		ContentType:   aws.String(contentType),
		Key:           aws.String(filename),
	}
	_, err = cli.PutObject(objectreq)
	if err != nil {
		log.Printf("Error: %v\n", err)
		return err
	} else {
		fmt.Printf("Uploaded https://%s.s3.amazonaws.com/%s\n", bucket, filename)
	}

	return nil
}

func listS3Bucket(config AwsConfig, prefix string) ([]string, error) {
	creds := aws.Creds(config.AccessKey, config.SecretKey, "")
	cli := s3.New(creds, config.Region, nil)
	bucket := config.Bucket

	listreq := s3.ListObjectsRequest{
		Bucket: aws.StringValue(&bucket),
	}
	listresp, err := cli.ListObjects(&listreq)

	var result []string
	if err != nil {
		return result, err
	}

	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		for _, obj := range listresp.Contents {
			if len(prefix) == 0 || (len(prefix) > 0 && strings.HasPrefix(*obj.Key, prefix)) {
				result = append(result, *obj.Key)
			}
		}
	}

	return result, nil
}

func CORSMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {

		c.Writer.Header().Set("Content-Type", "application/json")
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT")
		if c.Request.Method == "OPTIONS" {
			c.String(204, "")
			return
		}

		c.Next()
	}
}

func startServer(config tomlConfig) {
	r := gin.Default()

	listCache := ListCache{}

	r.Use(CORSMiddleware())

	r.OPTIONS("/upload/file", func(g *gin.Context) {
		g.String(204, "")
	})

	r.POST("/upload/url", func(c *gin.Context) {
		url := c.Request.FormValue("url")
		prefix := c.Request.FormValue("prefix")

		uploaded := false

		var newFilename string
		var err error
		if strings.HasPrefix(url, "http") {
			newFilename, err = uploadPathOrURL(config.Aws, url, prefix)
			if err == nil {
				uploaded = true
			}
		}

		if config.Server.CacheEnabled && config.Server.CacheBustOnUpload {
			log.Println("Cache BUST (upload url)")
			listCache.timeout = 0
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
		//log.Println(header)
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
			listCache.timeout = 0
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
			makeRequest = now > listCache.timeout
		}

		var items []string
		var err error
		if makeRequest {
			items, err = listS3Bucket(config.Aws, prefix)

			if config.Server.CacheEnabled && err == nil {
				log.Println("Cache MISS")
				listCache.items = make([]string, len(items))
				copy(listCache.items, items)
				listCache.timeout = time.Now().Unix() + config.Server.CacheTTL
			}
		} else {
			items = make([]string, len(listCache.items))
			copy(items, listCache.items)
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

	r.Run(fmt.Sprintf(":%d", config.Server.Port))
}

// TODO: use .Short() and .Default()
var (
	app        = kingpin.New("s3pal", "A server + cli S3 tool for uploading and listing files")
	configPath = app.Flag("config", "The path to a  non-default location config file.").Default("s3pal.toml").String()

	uploadCmd    = app.Command("upload", "Upload a local or remote file to S3.")
	uploadPath   = uploadCmd.Arg("path_or_url", "Path of local file or URL of remote file to upload to s3").Required().String()
	uploadBucket = uploadCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").String()

	serverCmd    = app.Command("server", "Run a server for handling uploads to S3")
	serverPort   = serverCmd.Flag("port", "The port to the run the upload server on").Default("8080").Int()
	serverBucket = serverCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").String()

	listCmd    = app.Command("list", "List the contents of the bucket")
	listPrefix = listCmd.Flag("prefix", "Only list objects that have this prefix").String()
	listBucket = listCmd.Flag("bucket", "The S3 bucket for listing objects.").String()
)

func main() {

	parsed := kingpin.MustParse(app.Parse(os.Args[1:]))

	var config tomlConfig
	if _, err := toml.DecodeFile(*configPath, &config); err != nil {
		fmt.Printf("Error loading config file. %v\n", err)
		// TODO: print out URL of link to example config file.
		return
	}

	switch parsed {
	// Upload local file
	case uploadCmd.FullCommand():
		if len(*serverBucket) > 0 {
			config.Aws.Bucket = *serverBucket
		}

		fmt.Printf("Uploading '%s' to S3 Bucket '%s'...\n", *uploadPath, *uploadBucket)
		uploadPathOrURL(config.Aws, *uploadPath, "")

	// Start server
	case serverCmd.FullCommand():
		if *serverPort > 0 {
			config.Server.Port = *serverPort
		}

		if len(*serverBucket) > 0 {
			config.Aws.Bucket = *serverBucket
		}

		startServer(config)

	case listCmd.FullCommand():
		if len(*listBucket) > 0 {
			config.Aws.Bucket = *listBucket
		}

		items, err := listS3Bucket(config.Aws, *listPrefix)

		if err == nil {
			for _, item := range items {
				fmt.Println(item)
			}
			fmt.Printf("\n%v Objects\n", len(items))
		} else {
			fmt.Printf("Error listing bucket '%s': %v", *listBucket, err)
		}

	default:
		fmt.Println("For help run: s3pal help")
	}

}
