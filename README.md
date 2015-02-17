# s3pal

A server + cli S3 tool for uploading and listing files.


You can run a simple server to handle uploads to s3 by running:

`./s3pal server`

You can upload a file on your computer

`./s3pal upload ~/Pictures/mycat.jpg`

or upload a file on the internet:

`./s3pal upload "https://www.google.com/images/srpr/logo11w.png"`

##Configuring

You configure `s3pal` using a toml config file. `s3pal` automatically looks for `s3pal.toml` in the working directory. Alternatively you can use the `--config` flag to provide a path.

#### Example config
	[aws]
	access_key = "AKI..."
	secret_key = "Iw3..."
	bucket = "mybucket"
	region = "us-west-2"

	[server]
	port = 8080


## Building

Assuming you have a proper go environment setup this should be as easy as:

    git clone git@github.com:jacktasia/s3pal.git
    cd s3pal
	go get ./... # install dependencies
	go build s3pal.go
	./s3pal help # test help
	#cp sample_s3pal.toml s3pal.toml
	#emacs s3pal.toml