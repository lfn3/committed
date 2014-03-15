package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"
	"path/filepath"

	"github.com/ghthor/journal/git"
	"github.com/howeyc/fsnotify"
)

type Config struct {
	WatchDirs []DirConfig
}

type DirConfig struct {
	Base         string
	SubDirs      []string
	IncludeFiles []string
	ExcludeFiles []string
}

const BUFFERLEN = 16

func main() {
	file, err := os.Open("./config.json")
	if err != nil {
		log.Fatal(err)
	}

	decoder := json.NewDecoder(file)
	config := &Config{}
	err = decoder.Decode(&config)
	if err != nil {
		log.Fatal(err)
	}

	file.Close()

	done := make(chan bool)

	watchers := make([]*fsnotify.Watcher, len(config.WatchDirs))

	for i, dir := range config.WatchDirs {
		log.Println("Created watcher on " + dir.Base)
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Fatal(err)
		}

		toBeCommitted := make(chan string, BUFFERLEN)

		// Process events
		go func() {
			for {
				select {
				case ev := <-watcher.Event:

					lowername := strings.ToLower(filepath.Base(ev.Name))
					includeFile := true

					//Check if a file is in the excluded list
					for _, exclude := range dir.ExcludeFiles {
						exclude = strings.ToLower(exclude)
						if strings.HasPrefix(exclude, "*") {
							exclude = strings.TrimPrefix(exclude, "*")
							if strings.HasSuffix(lowername, exclude) {
								log.Println(ev.Name + " was excluded due to rule: *" + exclude)
								includeFile = false
								break
							}
						} else if exclude == lowername {
							log.Println(ev.Name + " was excluded due to rule: " + exclude)
							includeFile = false
							break
						}
					}

					if includeFile {
						log.Println("Queuing commit for: " + ev.Name)
						toBeCommitted <- ev.Name
					}

				case err := <-watcher.Error:
					log.Println("error:", err)
				}
			}
		}()

		err = watcher.Watch(dir.Base)
		if err != nil {
			log.Fatal(err)
		}

		for _, subDir := range dir.SubDirs {
			err = watcher.Watch(subDir)
			if err != nil {
				log.Fatal(err)
			}
		}

		watchers[i] = watcher

		go CommitChanges(dir.Base, toBeCommitted)
	}

	<-done

	for i := 0; i < len(watchers); i++ {
		watchers[i].Close()
	}
}

const TIMERLEN = time.Second * 5

func CommitChanges(path string, fileQueue chan (string)) {
	changes := git.NewChangesIn(path)

	var timer *time.Timer

	for {
		select {
		case fileName := <-fileQueue:
			changes.Add(git.ChangedFile(fileName))

			if timer == nil {
				timer = time.NewTimer(TIMERLEN)

				go func() {
					for {
						<- timer.C
						log.Println("Committing:")

						for _, change := range changes.Changes() {
							log.Println("\t" + change.Filepath())
						}

						changes.Msg = "Updated"

						err := changes.Commit()
						if err != nil {
							log.Fatal(err)
						}

						changes = git.NewChangesIn(path)
					}
				}()

			} else {
				log.Println("Additional changes. Delaying...")
				timer.Reset(TIMERLEN)
			}
		}
	}
}
