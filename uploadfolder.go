package main

import (
	"fmt"
	"github.com/atotto/clipboard"
	"gopkg.in/fsnotify.v1"
	"log"
	"os"
	"time"
)

type FileReadyChecker struct {
	DelayTickChan   map[string]*time.Ticker
	LastFileDetails map[string]*FileDetails
	Debug           bool
	S3pal           *S3pal
}

type FileDetails struct {
	Name     string
	Size     int64
	Readable bool
}

func getFileDetails(path string) *FileDetails {
	f, err := os.Open(path)
	result := &FileDetails{
		Name:     path,
		Readable: false,
	}
	if err == nil {
		var stat os.FileInfo
		stat, err = f.Stat()
		if err == nil {
			result.Readable = true
			result.Size = stat.Size()
		}
		f.Close()
	}

	return result
}

func (o *FileReadyChecker) checkFile(path string) {

	if val, ok := o.DelayTickChan[path]; ok {
		val.Stop()
	}

	fwConfig := o.S3pal.Config.FolderWatchUpload
	o.LastFileDetails[path] = getFileDetails(path)

	//log.Println("Size now:", *o.LastFileDetails[path])

	o.DelayTickChan[path] = time.NewTicker(2 * time.Second)

	go func() {
		for {
			select {
			case <-o.DelayTickChan[path].C:
				now := getFileDetails(path)

				if now.Readable && now.Size == o.LastFileDetails[path].Size {
					o.DelayTickChan[path].Stop()

					newFilename, err := o.S3pal.uploadPathOrURL(path, fwConfig.Prefix)
					if err == nil {

						if fwConfig.AutoDeleteFile {
							fmt.Printf("\nAuto deleting '%v'...", path)
							err = os.Remove(path)
							if err != nil {
								fmt.Printf("Error! Not removed.")
							} else {
								fmt.Printf("Done.")
							}
						}

						if fwConfig.AutoClipboard {
							var toCopy string

							if len(fwConfig.AutoClipboardPrefix) > 0 {
								toCopy = fwConfig.AutoClipboardPrefix + newFilename
							} else {
								toCopy = o.S3pal.makeUrl(newFilename)
							}

							clipboard.WriteAll(toCopy)
							fmt.Printf("\nAdded '%v' to your clipboard\n\n", toCopy)
						}

					} else {
						fmt.Printf("\nError uploading '%v'\n\n", path)
					}

				} else {
					o.LastFileDetails[path] = now
				}
			}
		}
	}()
}

func (o *FileReadyChecker) startWatcher() {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	done := make(chan bool)
	go func() {
		for {
			select {
			case event := <-watcher.Events:
				if event.Op&fsnotify.Rename != fsnotify.Rename {
					o.checkFile(event.Name)
				}

			case err := <-watcher.Errors:
				log.Println("error:", err)
			}
		}
	}()

	err = watcher.Add(o.S3pal.Config.FolderWatchUpload.Path)
	fmt.Printf("\nLooking for new files in '%v'...\n", o.S3pal.Config.FolderWatchUpload.Path)
	if err != nil {
		log.Fatal(err)
	}
	<-done
}

func (s *S3pal) startDropFolder() {
	path := s.Config.FolderWatchUpload.Path
	if len(path) == 0 {
		fmt.Printf("\nNot Running! No Path defined in config or command line.\n\n")
		return
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		fmt.Printf("\nNot Running! Could not find folder '%v'\n\n", path)
		return
	}

	if !fileInfo.IsDir() {
		fmt.Printf("\nNot Running! '%v' is NOT a folder.\n\n", path)
		return
	}

	o := FileReadyChecker{
		DelayTickChan:   map[string]*time.Ticker{},
		LastFileDetails: map[string]*FileDetails{},
		S3pal:           s,
	}

	o.startWatcher()
}
