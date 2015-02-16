package main

import (
	"code.google.com/p/go-uuid/uuid"
	"encoding/json"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/gen/s3"
	"github.com/gin-gonic/gin"
	"io"
	"io/ioutil"
	"log"
	"os"
)

const MAX_UPLOAD_SIZE = 1000000

type tomlConfig struct {
	Aws    AwsConfig
	Server ServerConfig
}

type ServerConfig struct {
	Port int `toml:"port"`
}

type AwsConfig struct {
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
	Bucket    string
	Region    string
}

func uploadToS3(config AwsConfig, path string, contentType string, filename string) (err error) {
	fd, err := os.Open(path)
	if err != nil {
		panic(err)
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
		log.Printf("https://%s.s3.amazonaws.com/%s\n", bucket, filename)
	}

	return nil
}

func listS3Bucket(config AwsConfig) []string {
	creds := aws.Creds(config.AccessKey, config.SecretKey, "")
	cli := s3.New(creds, config.Region, nil)
	bucket := config.Bucket

	listreq := s3.ListObjectsRequest{
		Bucket: aws.StringValue(&bucket),
	}
	listresp, err := cli.ListObjects(&listreq)
	if err != nil {
		panic(err)
	}

	var result []string
	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		log.Printf("Content of bucket '%s': %d files\n", bucket, len(listresp.Contents))
		for _, obj := range listresp.Contents {
			result = append(result, *obj.Key)
		}
	}

	return result
}

func main() {

	var config tomlConfig
	if _, err := toml.DecodeFile("s3pal.toml", &config); err != nil {
		log.Println(err)
		return
	}

	r := gin.Default()
	r.POST("/uploader", func(c *gin.Context) {
		//c.String(200, "pong")
		file, header, err := c.Request.FormFile("file")
		//log.Println(header)

		if err != nil {
			return
		}

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

		newFilename := uuid.NewUUID().String()
		path := out.Name()
		out.Close()
		uploaded := false
		tooBig := fi.Size() > MAX_UPLOAD_SIZE

		if !tooBig {
			err := uploadToS3(config.Aws, path, header.Header.Get("Content-Type"), newFilename)

			if err == nil {
				uploaded = true
			} else {
				log.Println(err)
			}
		}

		if tooBig {
			c.String(400, fmt.Sprintf("%v > %v", fi.Size(), MAX_UPLOAD_SIZE))
		} else if uploaded {
			c.String(200, newFilename)
		} else {
			c.String(500, "NOT UPLOADED")
		}
	})

	r.GET("/list", func(c *gin.Context) {
		items := listS3Bucket(config.Aws)

		b, err := json.Marshal(items)
		if err == nil {
			c.String(200, string(b))
		} else {
			c.String(500, fmt.Sprintf("%v", err))
		}
	})

	r.Run(fmt.Sprintf(":%d", config.Server.Port))
}
