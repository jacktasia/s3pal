s3pal [![Build Status](https://travis-ci.org/jacktasia/s3pal.svg?branch=master)](https://travis-ci.org/jacktasia/s3pal)
========


A command line tool to help developers interact with s3.

* uploading of local files and urls
* run a server for easy browser uploading/listing (with configurable CORS)
* watch a folder for new files and have them be auto uploaded
* it's Go so you only need the binary
* easy to extend for your own needs (or PRs welcome)

Here's [an example](http://jackangers.com/imgix-wall) of me using the server feature.

* [Overview](#overview)
* [Configuring](#configuring)
* [Installing](#installing)
* [Building](#building)
* [Notes and Warnings](#notes_warnings)


<a name="overview"></a>
## Overview


### `s3pal watch-folder <folder>`

Watch a folder for new files and upload them to S3. There are options to auto delete and copy URL (see configuring).

### `s3pal upload <path>`

Upload a file on your computer like `s3pal upload ~/Pictures/mycat.jpg`

### `s3pal upload <url>`

Upload a file on the internet: `s3pal upload "https://www.google.com/images/srpr/logo11w.png"`

### `s3pal server`

A simple server to handle uploads to s3 by running:

This makes the following endpoints available:

**Upload a file**
* `POST /upload/file`
* Parameters: `file` `prefix`

Example HTML for uploads to this endpoint:

	 <form action="http://localhost:8080/upload/file" method="post" enctype="multipart/form-data">
		 <label for="file">Filename:</label>
		 <input type="file" name="file" id="file">
		 <input type="submit" name="submit" value="Submit">
	 </form>

**Upload a file from a url**
* `POST /upload/url`
* Parameters: `url` `prefix`

**List the contents of the bucket**
* `GET /list`
* Parameters: '?prefix'

**Simple embedded upload form**
* `GET /`
* Serves HTML upload form.



<a name="configuring"></a>
##Configuring

You configure `s3pal` using a toml config file. `s3pal` automatically looks for `s3pal.toml` in the working directory. If it's not there then it checks the user's home directory `~`. Alternatively you can use the `--config` flag to provide a path. Everything except for the s3 section is optional. Most values can be set/overriden on the command line.

#### Example config
	[aws]
	access_key = "AKI..."
	secret_key = "Iw3..."
	bucket = "mybucket"
	region = "us-west-2"
	upload_name_format="uploads/%Y/%M/%D/%N_%T%E" # this is the default

	# config below is all optional

	[aws.upload_headers]
	Cache-Control= "max-age=86400"
	x-amz-meta-test= "value" # must use x-amz-meta- for non-standard or s3 will drop it

	[server]
	port = 8080 # this is the default
	cache_enabled = true # defaults to false
	cache_bust_on_upload = true # defaults to false
	cache_ttl = 10
	max_post_bytes = 3000000 # ~3MB (unlimited if not set)
	static_path="/home/jack/assets" # directory served from /static (optional)
	allowed_origins=["http://jackangers.com", "http://blah.com"] # for cors. open "*" if unset

	[folderwatchupload]
	path = "/Users/jack/Desktop/toS3" # or pass in command line
	auto_clipboard = true   # defaults to false
	auto_delete_file = true # defaults to false

##### `upload_name_format` options

The `upload_name_format` option lets you control how uploaded files will be created in your bucket.

If this is unset it defaults to `"uploads/%Y/%M/%D/%N_%T%E"` which means if a file named `mycat.jpg` is uploaded on March 26, 2014 it will create a key like this `uploads/2014/03/26/mycat_1395792362.jpg`.

| directive   | meaning  | example  |
|---|---|---|
| `%F` | filename with extension | `cat.jpg` |
| `%N` | filename name (without extension) | `cat` |
| `%E` | extension of uploaded filename  | `.jpg` |
|`%T` | unix timestamp | `1425254762` |
|`%Y` | current year in 4 digits | `2015` |
|`%M` | current month in 2 digits | `04` |
|`%D` | current day in 2 digits | `09` |
|`%U` | a UUID | `0228a689-b578-11e4-b56c-0090f5c994d5` |

<a name="installing"></a>
## Installing

Download the static binary from [releases](https://github.com/jacktasia/s3pal/releases) or build a binary yourself (see **Building** below). Either way, ensure your `~/.profile` (mac) or `~/.bashrc` (linux) has the s3pal binary's directory in your path:

    export PATH=$PATH:/path/to/s3pal


<a name="building"></a>
## Building

If you have a proper `go` environment setup then it should be as easy as:

    git clone git@github.com:jacktasia/s3pal.git
    cd s3pal
	go get ./... # install dependencies
	go build
	./s3pal help # test help
	#cp sample_s3pal.toml s3pal.toml
	#emacs s3pal.toml
	#./s3pal server

<a name="notes_warnings"></a>
## Notes and Warnings

* This was initially made for quick and easy proof of concepts. [Here's a demo using it](http://jackangers.com/imgix-wall).
* This is untested for high load production environments.
* Please be careful and [open an issue](https://github.com/jacktasia/s3pal/issues) if you notice one.
* [Pull Requests](https://github.com/jacktasia/s3pal/pulls) are welcome.
