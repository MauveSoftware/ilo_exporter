package client

type Client interface {
	HostName() string
	Get(ressource string, obj interface{}) error
}
