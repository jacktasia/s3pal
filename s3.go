package main

import (
	"fmt"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

func (s *S3pal) getBucket() *s3.Bucket {
	auth, err := aws.GetAuth(s.Config.Aws.AccessKey, s.Config.Aws.SecretKey)

	if err != nil {
		panic("Could not connect to S3 with your credentials.")
	}

	client := s3.New(auth, aws.Regions[s.Config.Aws.Region])
	bucket := client.Bucket(s.Config.Aws.Bucket)

	return bucket
}

func (s *S3pal) uploadToS3(path string, contentType string, filename string) (err error) {
	fd, err := os.Open(path)
	if err != nil {
		log.Printf("Error opening temp: %v", err)
		return err
	}

	defer fd.Close()

	if len(contentType) == 0 {
		contentType = "binary/octet-stream"
	}

	bucket := s.getBucket()

	bytes, readErr := ioutil.ReadAll(fd)

	if readErr != nil {
		return readErr
	}

	headers := map[string][]string{
		"Content-Type": []string{contentType},
	}

	for key, value := range s.Config.Aws.UploadHeaders {
		headers[key] = []string{value}
	}

	err = bucket.PutHeader(filename, bytes, headers, s3.ACL(s.Config.Aws.ACL))

	if err != nil {
		log.Printf("Error: %v\n", err)
		return err
	} else {
		fmt.Printf("Uploaded %s\n", s.makeUrl(filename))
	}

	return nil
}

func (s *S3pal) listS3Bucket(prefix string, urls bool, doSign bool, signTTL int64) ([]string, error) {

	bucket := s.getBucket()
	listresp, err := bucket.List(prefix, "", "", 0)

	var result []string
	if err != nil {
		return result, err
	}

	now := time.Now()
	ttl := time.Duration(signTTL) * time.Second
	signExpires := now.Add(ttl)

	if err != nil {
		log.Printf("Error: %v\n", err)
	} else {
		for _, obj := range listresp.Contents {
			if len(prefix) == 0 || (len(prefix) > 0 && strings.HasPrefix(obj.Key, prefix)) {
				if urls {
					if doSign {
						result = append(result, bucket.SignedURL(obj.Key, signExpires))
					} else {
						result = append(result, bucket.URL(obj.Key))
					}
				} else {
					result = append(result, obj.Key)
				}
			}
		}
	}

	return result, nil
}

func (s *S3pal) uploadPathOrURL(filePath string, prefix string) (string, error) {
	fmt.Printf("\nUploading '%s' to S3 Bucket '%s'...\n", filePath, s.Config.Aws.Bucket)
	var toUploadPath string

	_, err := os.Stat(filePath)
	if err == nil {
		f, err := os.Open(filePath)
		if err != nil {
			return "", err
		}
		toUploadPath = f.Name()
	} else {
		toUploadPath, err = downloadURL(filePath)
		if err != nil {
			return "", err
		}
	}

	bytes, err := ioutil.ReadFile(toUploadPath)
	contentType := http.DetectContentType(bytes)
	newFilename := s.makeFilename(prefix, path.Base(filePath))

	err = s.uploadToS3(toUploadPath, contentType, newFilename)

	return newFilename, err
}
