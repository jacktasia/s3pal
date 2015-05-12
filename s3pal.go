package main

import (
	"code.google.com/p/go-uuid/uuid"
	"fmt"
	"github.com/BurntSushi/toml"
	"github.com/mitchellh/goamz/aws"
	"github.com/mitchellh/goamz/s3"
	"gopkg.in/alecthomas/kingpin.v1"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path"
	"strconv"
	"strings"
	"time"
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
	SignTTL           int64  `toml:"sign_ttl"`
	SignURL           bool   `toml:"sign_url"`
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
	AccessKey        string `toml:"access_key"`
	SecretKey        string `toml:"secret_key"`
	Bucket           string
	Region           string
	ACL              string
	UploadNameFormat string `toml:"upload_name_format"`
}

type ListCache struct {
	items   map[string][]string
	timeout map[string]int64
}

type S3pal struct {
	Config S3palConfig
}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func strToBool(s string) bool {
	r, err := strconv.ParseBool(s)
	if err != nil {
		return false
	}

	return r
}

var ValidACLs = []string{"private", "public-read", "public-read-write", "authenticated-read", "bucket-owner-read", "bucket-owner-full-control"}

func IsValidACL(acl string) bool {
	return stringInSlice(acl, ValidACLs)
}

func (s *S3pal) makeUrl(filename string) string {
	subdomain := "s3"

	if s.Config.Aws.Region != "us-east" {
		subdomain = subdomain + "-" + s.Config.Aws.Region
	}

	return fmt.Sprintf("https://%s.amazonaws.com/%s/%s", subdomain, s.Config.Aws.Bucket, filename)
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

// %U (uuid) %F (full filename) %N (name only w/o extension) %E(extension) %T(unix timestamp)
func (s *S3pal) makeFilename(prefix string, filename string) string {
	now := time.Now()
	t := now.UTC()
	day := fmt.Sprintf("%02d", t.Day())
	month := fmt.Sprintf("%02d", t.Month())
	year := fmt.Sprintf("%d", t.Year())
	uuid := uuid.NewUUID().String()
	ts := strconv.FormatInt(now.Unix(), 10)

	ext := path.Ext(filename)
	name := strings.Replace(filename, ext, "", -1)

	format := s.Config.Aws.UploadNameFormat

	if len(format) == 0 {
		format = "uploads/%Y/%M/%D/%N_%T%E"
	}

	newFilename := strings.Replace(format, "%F", filename, -1)
	newFilename = strings.Replace(newFilename, "%N", name, -1)
	newFilename = strings.Replace(newFilename, "%E", ext, -1)

	newFilename = strings.Replace(newFilename, "%T", ts, -1)
	newFilename = strings.Replace(newFilename, "%Y", year, -1)
	newFilename = strings.Replace(newFilename, "%M", month, -1)
	newFilename = strings.Replace(newFilename, "%D", day, -1)
	newFilename = strings.Replace(newFilename, "%U", uuid, -1)

	return path.Join(prefix, newFilename)
}

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

	err = bucket.Put(filename, bytes, contentType, s3.ACL(s.Config.Aws.ACL))

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

var (
	app        = kingpin.New("s3pal", "A server + cli S3 tool for uploading and listing files")
	configPath = app.Flag("config", "The path to a  non-default location config file.").Default("s3pal.toml").Short('c').String()

	// upload
	uploadCmd    = app.Command("upload", "Upload a local or remote file to S3.")
	uploadPath   = uploadCmd.Arg("path_or_url", "Path of local file or URL of remote file to upload to s3").Required().String()
	uploadBucket = uploadCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").Short('b').String()
	uploadPrefix = uploadCmd.Flag("prefix", "S3 prefix to prepend to filename when uploading (if different from default)").String()

	// upload folder
	folderWatchUploadCmd    = app.Command("watch-folder", "When running new files added this folder will uploaded to s3.")
	folderWatchUploadPath   = folderWatchUploadCmd.Arg("path", "Folder to watch for new files.").String()
	folderWatchUploadBucket = folderWatchUploadCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").String()
	folderWatchUploadPrefix = folderWatchUploadCmd.Flag("prefix", "S3 prefix to prepend to filename when uploading (if different from default)").String()

	// server
	serverCmd        = app.Command("server", "Run a server for handling uploads to S3")
	serverPort       = serverCmd.Flag("port", "The port to the run the upload server on").Int()
	serverBucket     = serverCmd.Flag("bucket", "S3 bucket name to upload to (if different from default)").Short('b').String()
	serverHost       = serverCmd.Flag("host", "Host to use for embedded html form (defaults to localhost").Default("localhost").String()
	serverPrefix     = serverCmd.Flag("prefix", "Prefix to use when uploading").String()
	serverDebug      = serverCmd.Flag("debug", "Server runs in debug mode.").Bool()
	serverStaticPath = serverCmd.Flag("static-path", "Serve this directory on /static").String()

	// list
	listCmd     = app.Command("list", "List the contents of the bucket")
	listPrefix  = listCmd.Flag("prefix", "Only list objects that have this prefix").String()
	listBucket  = listCmd.Flag("bucket", "S3 bucket for listing objects.").Short('b').String()
	listUrls    = listCmd.Flag("url", "List full urls and not just key names").Bool()
	listSign    = listCmd.Flag("sign", "Sign the S3 urls").Bool()
	listSignTTL = listCmd.Flag("sign-ttl", "TTL for signed URLs").Default("300").Int64()
)

func Exists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func main() {
	parsed := kingpin.MustParse(app.Parse(os.Args[1:]))

	if !Exists(*configPath) {
		usr, err := user.Current()
		if err != nil {
			log.Fatal(err)
			return
		}
		*configPath = path.Join(usr.HomeDir, "s3pal.toml")
	}

	var config S3palConfig
	if _, err := toml.DecodeFile(*configPath, &config); err != nil {
		fmt.Printf("\nError loading config file. \"%v\"\n", err)
		fmt.Printf("\nSee example s3pal.toml file: %v\n\n", "https://github.com/jacktasia/s3pal/blob/master/sample_s3pal.toml")
		return
	}

	s3pal := &S3pal{
		Config: config,
	}

	if s3pal.Config.Aws.ACL == "" {
		s3pal.Config.Aws.ACL = "public-read"
	}

	if !IsValidACL(s3pal.Config.Aws.ACL) {
		fmt.Printf("\n\"%v\" is not a valid ACL.\n", s3pal.Config.Aws.ACL)
		fmt.Printf("\nValid ACL options are: %v\n", strings.Join(ValidACLs, ", "))
	}

	switch parsed {

	// upload local file or URL
	case uploadCmd.FullCommand():
		if len(*uploadBucket) > 0 {
			s3pal.Config.Aws.Bucket = *uploadBucket
		}

		_, err := s3pal.uploadPathOrURL(*uploadPath, *uploadPrefix)

		if err != nil {
			fmt.Printf("\nNot Uploaded! Error: %v\n\n", err)
		}

	// watch folder for new files and then upload
	case folderWatchUploadCmd.FullCommand():
		if len(*folderWatchUploadBucket) > 0 {
			s3pal.Config.Aws.Bucket = *folderWatchUploadBucket
		}

		if len(*folderWatchUploadPath) > 0 {
			s3pal.Config.FolderWatchUpload.Path = *folderWatchUploadPath
		}

		if len(*folderWatchUploadPrefix) > 0 {
			s3pal.Config.FolderWatchUpload.Prefix = *folderWatchUploadPrefix
		}

		s3pal.startDropFolder()

	// Start server
	case serverCmd.FullCommand():
		if *serverPort > 0 {
			s3pal.Config.Server.Port = *serverPort
		}

		// handle default here...
		if s3pal.Config.Server.Port == 0 {
			s3pal.Config.Server.Port = 8080
		}

		if len(*serverBucket) > 0 {
			s3pal.Config.Aws.Bucket = *serverBucket
		}

		if len(*serverHost) > 0 {
			s3pal.Config.Server.Host = *serverHost
		}

		if len(*serverPrefix) > 0 {
			s3pal.Config.Server.Prefix = *serverPrefix
		}

		if *serverDebug {
			s3pal.Config.Server.Debug = *serverDebug
		}

		if len(*serverStaticPath) > 0 {
			s3pal.Config.Server.StaticPath = *serverStaticPath
		}

		port := forcePort(s3pal.Config.Server.Port)
		if port != s3pal.Config.Server.Port && s3pal.Config.Server.NoForcePort {
			fmt.Printf("\nNot Running! Port %v already in use.\n\n", s3pal.Config.Server.Port)
			fmt.Printf("'no_force_port' option is enabled.\n\n")

			return
		}

		s3pal.Config.Server.Port = port

		s3pal.startServer()

	// list
	case listCmd.FullCommand():
		if len(*listBucket) > 0 {
			s3pal.Config.Aws.Bucket = *listBucket
		}

		items, err := s3pal.listS3Bucket(*listPrefix, *listUrls, *listSign, *listSignTTL)

		if err == nil {
			for _, item := range items {
				fmt.Println(item)
			}
			fmt.Printf("\n%v Objects\n", len(items))
		} else {
			fmt.Printf("Error listing bucket '%s': %v", s3pal.Config.Aws.Bucket, err)
		}

	default:
		fmt.Println("For help run: s3pal help")
	}

}
