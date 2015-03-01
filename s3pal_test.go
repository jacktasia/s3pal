package main

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"strconv"
	"testing"
	"time"
)

func getS3palWithFormat(formatStr string) *S3pal {
	awsConfig := AwsConfig{
		UploadNameFormat: formatStr,
	}
	config := S3palConfig{
		Aws: awsConfig,
	}
	s3pal := &S3pal{
		Config: config,
	}

	return s3pal
}

func TestUploadNameF(t *testing.T) {
	s3pal := getS3palWithFormat("/blah/%F")

	result := s3pal.makeFilename("", "test.jpg")
	expected := "/blah/test.jpg"
	assert.Equal(t, result, expected)
}

func TestUploadNameTsExt(t *testing.T) {
	s3pal := getS3palWithFormat("/ts/%N_%T%E")

	result := s3pal.makeFilename("", "cat.jpg")

	ts := strconv.FormatInt(time.Now().Unix(), 10)
	expected := fmt.Sprintf("/ts/cat_%s.jpg", ts)
	assert.Equal(t, result, expected)
}

func TestWithYMD(t *testing.T) {
	s3pal := getS3palWithFormat("/images/%Y/%M/%D/animals.png")

	now := time.Now()
	ts := now.UTC()
	day := fmt.Sprintf("%02d", ts.Day())
	month := fmt.Sprintf("%02d", ts.Month())
	year := fmt.Sprintf("%d", ts.Year())

	result := s3pal.makeFilename("", "animals.jpg")
	expected := fmt.Sprintf("/images/%s/%s/%s/animals.png", year, month, day)
	assert.Equal(t, result, expected)
}

func TestWithUUID(t *testing.T) {
	s3pal := getS3palWithFormat("uuid/%U")

	result := s3pal.makeFilename("", "table.jpg")
	assert.Equal(t, len(result), 5+36)
}
