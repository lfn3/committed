package main

import (
	"encoding/json"
	"log"
	"os"
	"strings"
	"time"

	"github.com/ghthor/journal/git"
	"github.com/howeyc/fsnotify"
)

/* TODO:
Pop changes from channel, add to git list thing
delay doohicky
*/
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

					lowername := strings.ToLower(ev.Name)
					//Check if a file is in the excluded list
					for _, exclude := range dir.ExcludeFiles {
						if strings.HasPrefix(exclude, "*") {
							exclude = strings.TrimPrefix(exclude, "*")
							if strings.HasSuffix(lowername, exclude) {
								log.Println(lowername + " was excluded due to rule: *" + exclude)
								return
							}
						} else if exclude == lowername {
								log.Println(lowername + " was excluded due to rule: " + exclude)
							return
						}
					}

					toBeCommitted <- ev.Name

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

const TIMERLEN = 500

func CommitChanges(path string, fileQueue chan (string)) {
	changes := git.NewChangesIn(path)

	var timer *time.Timer

	select {
	case fileName := <-fileQueue:
		changes.Add(git.ChangedFile(fileName))

		if timer == nil {
			timer = time.AfterFunc(TIMERLEN, func() {

				log.Println("Committing:")

				for _, change := range changes.Changes() {
					log.Println(change)
				}

				changes.Msg = "Test!"

				err := changes.Commit()
				if err != nil {
					log.Fatal(err)
				}
			})
		} else {
			timer.Reset(TIMERLEN)
		}
	}

}
