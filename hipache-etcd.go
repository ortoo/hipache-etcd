package main

import (
	"fmt"
	"strings"
	"os"

	"github.com/coreos/go-etcd/etcd"
	"github.com/garyburd/redigo/redis"

	"github.com/ortoo/hipache-etcd/clients"
)

var etcdWatchKey string

func WatchServices(receiver chan *etcd.Response) {
	client := clients.EtcdClient()

	if client == nil {
		fmt.Println("Error connecting to etcd")
	}

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

// On any change we just refresh everything
func handleChange(etcdClient *etcd.Client) {

	resp, err := etcdClient.Get(etcdWatchKey, false, true)
	if err != nil {
		fmt.Println(err)
		return
	}

	// Go and fetch all the services in REDIS
	redisClient := clients.RedisClient()
	if (redisClient == nil) {
		fmt.Println("Couldn't connect to redis")
		return
	}
	defer redisClient.Close()

	keys, err := redis.Strings(redisClient.Do("KEYS", "frontend:*"))
	if err != nil {
		fmt.Println(err)
		return
	}

	// Now we delete everything in redis and add everything back in from etcd
	redisClient.Send("MULTI")
	if len(keys) > 0 {
		redisClient.Send("DEL", redis.Args{}.AddFlat(keys)...)
	}

	for _, node := range resp.Node.Nodes {
		// This first level is a frontend
		split := strings.Split(node.Key, "/")
		domain := split[len(split) - 1]

		frontendKey := "frontend:" + domain

		// Belt and braces - delete this key just in case its not already (but it should be)
		redisClient.Send("DEL", frontendKey)
		redisClient.Send("RPUSH", frontendKey, domain)

		for _, instNode := range node.Nodes {
			instsplit := strings.Split(instNode.Key, "/")
			inst := instsplit[len(instsplit) - 1]
			host := "http://" + inst
			redisClient.Send("RPUSH", frontendKey, host)
		}
	}

	// Should be everything
	_, err = redisClient.Do("EXEC")
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println("Resynced hipache from etcd")
}

func HandleServiceChanges(receiver chan *etcd.Response) {
	// Do an initial sync and then listen for any messages on the receiver
	// Go and fetch all the services from etcd
	etcdClient := clients.EtcdClient()

	if etcdClient == nil {
		fmt.Println("Couldn't connect to etcd")
		return
	}

	handleChange(etcdClient)
	for _ = range receiver {
		handleChange(etcdClient)
	}
}

func main() {

	etcdWatchKey = os.Getenv("ETCD_WATCH_KEY")
	if etcdWatchKey == "" {
		etcdWatchKey = "/services"
	}

	// Watch the services directory for changes
	etcdchan := make(chan *etcd.Response)
	go WatchServices(etcdchan)
	HandleServiceChanges(etcdchan)
}
