package goroslib

import (
	"fmt"
	"net"
	"time"

	"github.com/aler9/goroslib/pkg/apislave"
)

// InfoNode contains informations about a node.
type InfoNode struct {
	PublishedTopics  map[string]struct{}
	SubscribedTopics map[string]struct{}
	ProvidedServices map[string]struct{}
	Address          string
}

// GetNodes returns all the nodes connected to the master.
func (n *Node) GetNodes() (map[string]*InfoNode, error) {
	res, err := n.apiMasterClient.GetSystemState()
	if err != nil {
		return nil, err
	}

	ret := make(map[string]*InfoNode)

	initEntry := func(node string) {
		if _, ok := ret[node]; !ok {
			ret[node] = &InfoNode{
				PublishedTopics:  make(map[string]struct{}),
				SubscribedTopics: make(map[string]struct{}),
				ProvidedServices: make(map[string]struct{}),
			}
		}
	}

	for _, entry := range res.State.PublishedTopics {
		for _, node := range entry.Nodes {
			initEntry(node)
			ret[node].PublishedTopics[entry.Name] = struct{}{}
		}
	}

	for _, entry := range res.State.SubscribedTopics {
		for _, node := range entry.Nodes {
			initEntry(node)
			ret[node].SubscribedTopics[entry.Name] = struct{}{}
		}
	}

	for _, entry := range res.State.ProvidedServices {
		for _, node := range entry.Nodes {
			initEntry(node)
			ret[node].ProvidedServices[entry.Name] = struct{}{}
		}
	}

	for nodeName, info := range ret {
		res, err := n.apiMasterClient.LookupNode(nodeName)
		if err != nil {
			return nil, fmt.Errorf("lookupNode: %v", err)
		}

		address, err := urlToAddress(res.URL)
		if err != nil {
			return nil, err
		}

		info.Address = address
	}

	return ret, nil
}

// GetMachines returns all the machines connected to the master with a node.
func (n *Node) GetMachines() (map[string]struct{}, error) {
	// this is like its equivalent in python
	// https://docs.ros.org/melodic/api/rosnode/html/rosnode-pysrc.html#get_machines_by_nodes

	nodes, err := n.GetNodes()
	if err != nil {
		return nil, err
	}

	ret := make(map[string]struct{})
	for _, info := range nodes {
		host, _, err := net.SplitHostPort(info.Address)
		if err != nil {
			continue
		}
		ret[host] = struct{}{}
	}

	return ret, nil
}

// InfoTopic contains informations about a topic.
type InfoTopic struct {
	Type        string
	Publishers  map[string]struct{}
	Subscribers map[string]struct{}
}

// GetTopics returns all the topics published by nodes connected to the master.
func (n *Node) GetTopics() (map[string]*InfoTopic, error) {
	res1, err := n.apiMasterClient.GetSystemState()
	if err != nil {
		return nil, fmt.Errorf("getSystemState: %v", err)
	}

	res2, err := n.apiMasterClient.GetTopicTypes()
	if err != nil {
		return nil, fmt.Errorf("getTopicTypes: %v", err)
	}

	ret := make(map[string]*InfoTopic)

	for _, entry := range res2.Types {
		ret[entry.Name] = &InfoTopic{
			Type:        entry.Type,
			Publishers:  make(map[string]struct{}),
			Subscribers: make(map[string]struct{}),
		}
	}

	for _, entry := range res1.State.PublishedTopics {
		if _, ok := ret[entry.Name]; !ok {
			continue
		}
		for _, node := range entry.Nodes {
			ret[entry.Name].Publishers[node] = struct{}{}
		}
	}

	for _, entry := range res1.State.SubscribedTopics {
		if _, ok := ret[entry.Name]; !ok {
			continue
		}
		for _, node := range entry.Nodes {
			ret[entry.Name].Subscribers[node] = struct{}{}
		}
	}

	return ret, nil
}

// InfoService contains informations about a service.
type InfoService struct {
	Providers map[string]struct{}
	Address   string
}

// GetServices returns all the services provided by nodes connected to the server.
func (n *Node) GetServices() (map[string]*InfoService, error) {
	res1, err := n.apiMasterClient.GetSystemState()
	if err != nil {
		return nil, fmt.Errorf("getSystemState: %v", err)
	}

	ret := make(map[string]*InfoService)

	for _, entry := range res1.State.ProvidedServices {
		if _, ok := ret[entry.Name]; !ok {
			ret[entry.Name] = &InfoService{
				Providers: make(map[string]struct{}),
			}
		}

		for _, node := range entry.Nodes {
			ret[entry.Name].Providers[node] = struct{}{}
		}

		res2, err := n.apiMasterClient.LookupService(entry.Name)
		if err != nil {
			return nil, fmt.Errorf("lookupService: %v", err)
		}

		address, err := urlToAddress(res2.URL)
		if err != nil {
			return nil, err
		}

		ret[entry.Name].Address = address
	}

	return ret, nil
}

// PingNode sends a ping request to a given node, wait for the response and returns
// the elapsed time.
func (n *Node) PingNode(nodeName string) (time.Duration, error) {
	res, err := n.apiMasterClient.LookupNode(nodeName)
	if err != nil {
		return 0, err
	}

	address, err := urlToAddress(res.URL)
	if err != nil {
		return 0, err
	}

	xcs := apislave.NewClient(address, n.absoluteName())

	start := time.Now()

	_, err = xcs.GetPid()
	if err != nil {
		return 0, err
	}

	return time.Since(start), nil
}

// KillNode sends a kill request to a given node.
func (n *Node) KillNode(nodeName string) error {
	res, err := n.apiMasterClient.LookupNode(nodeName)
	if err != nil {
		return err
	}

	address, err := urlToAddress(res.URL)
	if err != nil {
		return err
	}

	xcs := apislave.NewClient(address, n.absoluteName())

	err = xcs.Shutdown("")
	if err != nil {
		return err
	}

	return nil
}

// InfoConnection contains information about a connection.
type InfoConnection struct {
	ID        int
	To        string
	Direction byte
	Transport string
	Topic     string
	Connected bool
}

// GetNodeConns returns infos about connections of a node.
func (n *Node) GetNodeConns(nodeName string) ([]InfoConnection, error) {
	res, err := n.apiMasterClient.LookupNode(nodeName)
	if err != nil {
		return nil, err
	}

	address, err := urlToAddress(res.URL)
	if err != nil {
		return nil, err
	}

	xcs := apislave.NewClient(address, n.absoluteName())

	infos, err := xcs.GetBusInfo()
	if err != nil {
		return nil, err
	}

	var ret []InfoConnection

	for _, i := range infos {
		if len(i) < 6 {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}

		id, ok := i[0].(int)
		if !ok {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}

		counterpart, ok := i[1].(string)
		if !ok {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}

		temp, ok := i[2].(string)
		if !ok || len(temp) != 1 {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}
		direction := temp[0]

		transport, ok := i[3].(string)
		if !ok {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}

		topic, ok := i[4].(string)
		if !ok {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}

		connected, ok := i[5].(bool)
		if !ok {
			return nil, fmt.Errorf("invalid entry: %v", i)
		}

		ret = append(ret, InfoConnection{
			ID:        id,
			To:        counterpart,
			Direction: direction,
			Transport: transport,
			Topic:     topic,
			Connected: connected,
		})
	}

	return ret, nil
}
