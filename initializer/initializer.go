package initializer

import (
	"sync"
	"fmt"
	"strings"

	// "github.com/coreos/go-etcd/etcd"
	"github.com/garyburd/redigo/redis"

	"github.com/ortoo/hipache-etcd/clients"
)

type SyncingFrontend struct {
	cond *sync.Cond
	synced bool
	lock sync.Mutex
}

type FrontendStatus struct {
	syncedFrontends map[string]uint64
	syncedFrontendsLock sync.RWMutex

	syncingFrontends map[string]*SyncingFrontend
	syncingFrontendsLock sync.RWMutex

	gotFrontendList bool
	gotFrontendListCond *sync.Cond
	gotFrontendListLock sync.Mutex
}

var status *FrontendStatus

// Wait until we have got a list of the frontends that we are going to sync
func waitForFrontendList() {
	status.gotFrontendListLock.Lock()
	// If we dont have the list then wait until we do
	for !status.gotFrontendList {
		status.gotFrontendListCond.Wait()
	}
	// We have the list
	status.gotFrontendListLock.Unlock()
}

func waitForFrontendSync(frontend string) {
	status.syncingFrontendsLock.RLock()

	// If we are syncing the frontend then wait for it
	syncing, ok := status.syncingFrontends[frontend]
	if ok {
		syncing.lock.Lock()

		// If we are already synced we'll drop right through
		for !syncing.synced {
			syncing.cond.Wait()
		}
		syncing.lock.Unlock()
	}

	status.syncingFrontendsLock.RUnlock()
}

func getFrontendIndex(frontend string) uint64 {
	status.syncedFrontendsLock.RLock()
	val := status.syncedFrontends[frontend]
	status.syncedFrontendsLock.RUnlock()
	return val
}

func syncFrontends(key string) {
	client := clients.EtcdClient()
	resp, err := client.Get(key, false, false)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, node := range resp.Node.Nodes {
		syncFrontend(node.Key)
	}

	status.gotFrontendListLock.Lock()
	status.gotFrontendList = true
	status.gotFrontendListCond.Broadcast()
	status.gotFrontendListLock.Unlock()

	fmt.Println("Got frontend list")
}

func doSync(frontend string) {
	etcdClient := clients.EtcdClient()
	redisClient := clients.RedisClient()

	// Get a list of the frontends
	resp, err := etcdClient.Get(frontend, false, false)

	if err != nil {
		fmt.Println(err)
		return
	}

	split := strings.Split(frontend, "/")
	domain := split[2]
	redisKey := "frontend:" + domain

	// Do a MULTI UPDATE
	_, err = redisClient.Do("MULTI")
	if err != nil {
		fmt.Println(err)
		return
	}

	_, err = redisClient.Do("DEL", redisKey)
	if err != nil {
		fmt.Println(err)
		return
	}

	_, err = redisClient.Do("RPUSH", redisKey, domain)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, hostNode := range resp.Node.Nodes {
		split := strings.Split(hostNode.Key, "/")
		host := "http://" + split[3]
		_, err = redisClient.Do("RPUSH", redisKey, host)
		if err != nil {
			fmt.Println(err)
			return
		}
	}

	_, err = redisClient.Do("EXEC")
	if err != nil {
		fmt.Println(err)
		return
	}

	index := resp.EtcdIndex

	// Finish off by announcing that we are done
	status.syncedFrontendsLock.Lock()
	status.syncedFrontends[frontend] = index
	status.syncedFrontendsLock.Unlock()

	status.syncingFrontendsLock.Lock()
	syncing := status.syncingFrontends[frontend]
	syncing.lock.Lock()
	syncing.synced = true
	syncing.cond.Broadcast()
	syncing.lock.Unlock()
	status.syncingFrontendsLock.Unlock()
}

func syncFrontend(frontend string) {
	// Set up the store
	status.syncingFrontendsLock.Lock()
	fe := new(SyncingFrontend)
	fe.cond = sync.NewCond(&fe.lock)
	status.syncingFrontends[frontend] = fe
	status.syncingFrontendsLock.Unlock()

	go doSync(frontend)
}

func clearExistingConfig() {
	client := clients.RedisClient()
	keys, err := redis.Strings(client.Do("KEYS", "frontend:*"))
	if err != nil {
		fmt.Println(err)
		return
	}

	if len(keys) > 0 {
		_, err = client.Do("DEL", redis.Args{}.AddFlat(keys)...)

		if err != nil {
			fmt.Println(err)
			return
		}	
	}
	fmt.Println("Cleared existing frontend config")
}

func Initialize() {
	status = new(FrontendStatus)

	status.syncingFrontends = make(map[string]*SyncingFrontend)
	status.syncedFrontends = make(map[string]uint64)

	status.gotFrontendListCond = sync.NewCond(&status.gotFrontendListLock)
}

func SyncedIndex(frontend string) uint64 {
	waitForFrontendList()
	waitForFrontendSync(frontend)

	return getFrontendIndex(frontend)
}

func Sync(key string) {
	clearExistingConfig()
	syncFrontends(key)
}