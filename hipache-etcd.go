package main

import (
	"fmt"
	"strings"
	"os"

	"github.com/coreos/go-etcd/etcd"
	"github.com/garyburd/redigo/redis"

	"github.com/ortoo/hipache-etcd/initializer"
	"github.com/ortoo/hipache-etcd/clients"
)

var etcdWatchKey string

func WatchServices(receiver chan *etcd.Response) {
	client := clients.EtcdClient()

	// Set the directory
	client.SetDir(etcdWatchKey, 0)

	for {
		fmt.Printf("Created etcd watcher: key=%s\n", etcdWatchKey)

		_, err := client.Watch(etcdWatchKey, 0, true, receiver, nil)

		var errString string
		if err == nil {
			errString = "N/A"
		} else {
			errString = err.Error()
		}

		fmt.Printf("etcd watch exited: key=%s, err=\"%s\"\n", etcdWatchKey, errString)
	}
}

func setFrontend(frontendKey string, domain string, host string, redisClient redis.Conn) (reply interface{}, err error) {
	// Does the frontend already exist
	redisClient.Do("WATCH", frontendKey)
	exists, err := redis.Bool(redisClient.Do("EXISTS", frontendKey))

	if err != nil {
		return nil, err
	}

	redisClient.Do("MULTI")
	// Don't want duplicates
	if host != "" {
		redisClient.Do("LREM", frontendKey, 0, host)
	}

	if exists && host != "" {
		redisClient.Do("RPUSH", frontendKey, host)
	} else {
		redisClient.Do("RPUSH", frontendKey, domain)

		if host != "" {
			redisClient.Do("RPUSH", frontendKey, host)	
		}
	}

	return redisClient.Do("EXEC")
}

func handleChange(action string, node *etcd.Node, index uint64) {	
	syncedIndex := initializer.SyncedIndex(node.Key)

	// If the synced index is gte our one then just exit
	if syncedIndex != 0 && (syncedIndex >= index) {
		fmt.Println("Already synced this change:", action, node.Key)
		return
	}

	split := strings.Split(node.Key, "/")

	if len(split) < 3 {
		return
	}

	var host string
	domain := split[2]

	if len(split) == 4 {
		host = "http://" + split[3]	
	}

	frontendKey := "frontend:" + domain

	redisClient := clients.RedisClient()

	switch action {
	case "delete":
		if host == "" {
			redisClient.Do("DEL", frontendKey)
		} else {
			redisClient.Do("LREM", frontendKey, 0, host)
			fmt.Println("Deleted frontend", domain, host)
		}
	
	case "create":
		fallthrough
	case "set":
		var rep interface{} = nil

		// Repeat until we get a non-nil response
		for rep := rep; rep == nil; {
			var err error
			rep, err = setFrontend(frontendKey, domain, host, redisClient)
			if err != nil {
				fmt.Println(err)
				return
			}
		}

		fmt.Println("Added/updated frontend", domain, host)
	}
}

func HandleServiceChanges(receiver chan *etcd.Response) {
	for resp := range receiver {
		go handleChange(resp.Action, resp.Node, resp.EtcdIndex)
	}
}

func main() {

	etcdWatchKey = os.Getenv("ETCD_WATCH_KEY")
	if etcdWatchKey == "" {
		etcdWatchKey = "/services"
	}

	initializer.Initialize()

	// Watch the services directory for changes
	etcdchan := make(chan *etcd.Response)
	go WatchServices(etcdchan)

	// Kick off the initial sync
	go initializer.Sync(etcdWatchKey)

	HandleServiceChanges(etcdchan)
}
