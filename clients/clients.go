package clients

import (
	"fmt"
	"os"
	
	"github.com/coreos/go-etcd/etcd"
	"github.com/garyburd/redigo/redis"
)

func EtcdClient() *etcd.Client {
	host := os.Getenv("ETCD_HOST")
	port := os.Getenv("ETCD_PORT")

	if port == "" {
		port = "4001"
	}

	addr := "http://" + host + ":" + port
	addrs := []string{addr}

	return etcd.NewClient(addrs)
}

func RedisClient() redis.Conn {
	host := os.Getenv("DB_PORT_6379_TCP_ADDR")
	port := os.Getenv("DB_PORT_6379_TCP_PORT")
	proto := os.Getenv("DB_PORT_6379_TCP_PROTO")

	if port == "" {
		port = "6379"
	}

	if proto == "" {
		proto = "tcp"
	}

	addr := host + ":" + port

	client, err := redis.Dial(proto, addr)

	if err != nil {
		fmt.Println(err)
		return nil
	}

	return client
}
