package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/awslabs/aws-sdk-go/aws"
	"github.com/awslabs/aws-sdk-go/gen/s3"
	"gopkg.in/alecthomas/kingpin.v1"
	"io"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
)

type S3palConfig struct {
	Aws               AwsConfig
	Server            ServerConfig
	FolderWatchUpload FolderWatchUploadConfig
}

type ServerConfig struct {
	Port              int   `toml:"port"`
	MaxPostBytes      int64 `toml:"max_post_bytes"`
	CacheEnabled      bool  `toml:"cache_enabled"`
	CacheBustOnUpload bool  `toml:"cache_bust_on_upload"`
	CacheTTL          int64 `toml:"cache_ttl"`
	NoForcePort       bool  `toml:"no_force_port"`
	Host              string
	Prefix            string
	Debug             bool   `toml:"debug"`
	StaticPath        string `toml:"static_path"`
}

type FolderWatchUploadConfig struct {
	Path                string `toml:"path"`
	Prefix              string `toml:"prefix"`
	AutoDeleteFile      bool   `toml:"auto_delete_file"`
	AutoClipboard       bool   `toml:"auto_clipboard"`
	AutoClipboardPrefix string `toml:"auto_clipboard_prefix"`
	Debug               bool   `toml:"debug"`
}

type AwsConfig struct {
	AccessKey string `toml:"access_key"`
	SecretKey string `toml:"secret_key"`
	Bucket    string
	Region    string
}

type ListCache struct {
	items   map[string][]string
	timeout map[string]int64
}

func makeUrl(config AwsConfig, filename string) string {

	subdomain := "s3"

	if config.Region != "us-east" {
		subdomain = subdomain + "-" + config.Region
	}

	return fmt.Sprintf("https://%s.amazonaws.com/%s/%s", subdomain, config.Bucket, filename)
}

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

func downloadURL(url string) (string, error) {
	client := &http.Client{}
	req, err := http.NewRequest("GET", url, nil)

	if err != nil {
		return "", fmt.Errorf("Request error: %v", err)
	}

	req.Header.Set("User-Agent", "s3pal Downloader")
	resp, err := client.Do(req)

	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode > 399 {
		return "", fmt.Errorf("%v returned by %v", resp.StatusCode, url)
	}

	tmp, err := ioutil.TempFile("/tmp", "downloaded_")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	_, err = io.Copy(tmp, resp.Body)
	if err != nil {
		return "", err
	}

	return tmp.Name(), nil
}

func UploadPathOrURL(config AwsConfig, path string, prefix string) (string, error) {
	fmt.Printf("\nUploading '%s' to S3 Bucket '%s'...\n", path, config.Bucket)
	var toUploadPath string

	_, err := os.Stat(path)
	if err == nil {
		f, err := os.Open(path)
		if err != nil {
			return "", err
		}
		toUploadPath = f.Name()
	} else {
		toUploadPath, err = downloadURL(path)
		if err != nil {
			return "", err
		}
	}

	bytes, err := ioutil.ReadFile(toUploadPath)
	contentType := http.DetectContentType(bytes)
	newFilename := makeFilename(prefix)

	err = uploadToS3(config, toUploadPath, contentType, newFilename)

	return newFilename, err
}

func makeFilename(prefix string) string {
	return path.Join(prefix, uuid.NewUUID().String())
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
		fmt.Printf("Uploaded %s\n", makeUrl(config, filename))
	}

	return nil
}

func listS3Bucket(config AwsConfig, prefix string) ([]string, error) {
	creds := aws.Creds(config.AccessKey, config.SecretKey, "")
	cli := s3.New(creds, config.Region, nil)
	bucket := config.Bucket

	listreq := s3.ListObjectsRequest{
		Bucket: aws.StringValue(&bucket),
		Prefix: aws.StringValue(&prefix),
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

// TODO: use .Short() too
var (
	app        = kingpin.New("s3pal", "A server + cli S3 tool for uploading and listing files")
	configPath = app.Flag("config", "The path to a  non-default location config file.").Default("s3pal.toml").String()

	// upload
	uploadCmd    = app.Command("upload", "Upload a local or remote file to S3.")
	uploadPath   = uploadCmd.Arg("path_or_url", "Path of local file or URL of remote file to upload to s3").Required().String()
	uploadBucket = uploadCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").String()
	uploadPrefix = uploadCmd.Flag("prefix", "S3 prefix to prepend to filename when uploading (if different from default)").String()

	// upload folder
	folderWatchUploadCmd    = app.Command("watch-folder", "When running new files added this folder will uploaded to s3.")
	folderWatchUploadPath   = folderWatchUploadCmd.Arg("path", "Folder to watch for new files.").String()
	folderWatchUploadBucket = folderWatchUploadCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").String()
	folderWatchUploadPrefix = folderWatchUploadCmd.Flag("prefix", "S3 prefix to prepend to filename when uploading (if different from default)").String()

	// server
	serverCmd        = app.Command("server", "Run a server for handling uploads to S3")
	serverPort       = serverCmd.Flag("port", "The port to the run the upload server on").Default("8080").Int()
	serverBucket     = serverCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").String()
	serverHost       = serverCmd.Flag("host", "Host to use for embedded html form (defaults to localhost").Default("localhost").String()
	serverPrefix     = serverCmd.Flag("prefix", "Prefix to use when uploading").String()
	serverDebug      = serverCmd.Flag("debug", "Server runs in debug mode.").Bool()
	serverStaticPath = serverCmd.Flag("static-path", "Serve this directory on /static").String()

	// list
	listCmd    = app.Command("list", "List the contents of the bucket")
	listPrefix = listCmd.Flag("prefix", "Only list objects that have this prefix").String()
	listBucket = listCmd.Flag("bucket", "The S3 bucket for listing objects.").String()
)

func main() {
	parsed := kingpin.MustParse(app.Parse(os.Args[1:]))

	var config S3palConfig
	if _, err := toml.DecodeFile(*configPath, &config); err != nil {
		fmt.Printf("\nError loading config file. \"%v\"\n", err)
		fmt.Printf("\nSee example s3pal.toml file: %v\n\n", "https://github.com/jacktasia/s3pal/blob/master/sample_s3pal.toml")
		return
	}

	switch parsed {
	// Upload local file
	case uploadCmd.FullCommand():
		if len(*uploadBucket) > 0 {
			config.Aws.Bucket = *uploadBucket
		}

		_, err := UploadPathOrURL(config.Aws, *uploadPath, *uploadPrefix)

		if err != nil {
			fmt.Printf("\nNot Uploaded! Error: %v\n\n", err)
		}

	case folderWatchUploadCmd.FullCommand():
		if len(*folderWatchUploadBucket) > 0 {
			config.Aws.Bucket = *folderWatchUploadBucket
		}

		if len(*folderWatchUploadPath) > 0 {
			config.FolderWatchUpload.Path = *folderWatchUploadPath
		}

		if len(*folderWatchUploadPrefix) > 0 {
			config.FolderWatchUpload.Prefix = *folderWatchUploadPrefix
		}

		StartDropFolder(config)

	// Start server
	case serverCmd.FullCommand():
		if *serverPort > 0 {
			config.Server.Port = *serverPort
		}

		if len(*serverBucket) > 0 {
			config.Aws.Bucket = *serverBucket
		}

		if len(*serverHost) > 0 {
			config.Server.Host = *serverHost
		}

		if len(*serverPrefix) > 0 {
			config.Server.Prefix = *serverPrefix
		}

		if *serverDebug {
			config.Server.Debug = *serverDebug
		}

		if len(*serverStaticPath) > 0 {
			config.Server.StaticPath = *serverStaticPath
		}

		StartServer(config)

	// list
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
			fmt.Printf("Error listing bucket '%s': %v", config.Aws.Bucket, err)
		}

	default:
		fmt.Println("For help run: s3pal help")
	}

}
