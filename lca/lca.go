package lca

import (
    "errors"
    "context"
    "fmt"
    "sort"
    "sync"
    "time"

    "github.com/libp2p/go-libp2p"
    "github.com/libp2p/go-libp2p/p2p/protocol/ping"
    "github.com/libp2p/go-libp2p-core/host"
    "github.com/libp2p/go-libp2p-core/network"
    "github.com/libp2p/go-libp2p-core/peer"
    "github.com/libp2p/go-libp2p-core/protocol"
    "github.com/libp2p/go-libp2p-discovery"

    "github.com/libp2p/go-libp2p-kad-dht"
    "github.com/multiformats/go-multiaddr"
)


var ErrUhOh = errors.New("Communication error with LCA")


var DefaultBootstrapPeers []multiaddr.Multiaddr
var DefaultListenAddresses []multiaddr.Multiaddr

var LCAServerProtocolID protocol.ID
var LCAServerRendezvous string

var LCAClientProtocolID protocol.ID
var LCAClientRendezvous string

func init() {
    for _, s := range []string{
        "/ip4/10.11.17.15/tcp/4001/ipfs/QmeZvvPZgrpgSLFyTYwCUEbyK6Ks8Cjm2GGrP2PA78zjAk",
    } {
        ma, err := multiaddr.NewMultiaddr(s)
        if err != nil {
            panic(err)
        }
        DefaultBootstrapPeers = append(DefaultBootstrapPeers, ma)
    }

    for _, s := range []string{
        "/ip4/0.0.0.0/tcp/4001",
    } {
        ma, err := multiaddr.NewMultiaddr(s)
        if err != nil {
            panic(err)
        }
        DefaultListenAddresses = append(DefaultListenAddresses, ma)
    }

    LCAServerProtocolID = protocol.ID("/lcaserver/1.1.0")
    LCAServerRendezvous = "QmQJRHSU69L6W2SwNiKekpUHbxHPXi57tWGRWJaD5NsRxS"

    LCAClientProtocolID = protocol.ID("/lcaclient/1.1.0")
    LCAClientRendezvous = "QmRJQHSU69L6W2SwNiKekpUHbxHPXi57tWGRWJaD5NsRxS"
}


type PeerInfo struct {
    RTT   time.Duration
    ID    peer.ID
    Addrs []multiaddr.Multiaddr
}

func SortPeers(peerChan <-chan peer.AddrInfo, lcaHost LCAHost) []PeerInfo {
	var peers []PeerInfo

    for p := range peerChan {
        responseChan := ping.Ping(lcaHost.Ctx, lcaHost.Host, p.ID)
        result := <-responseChan
        if len(p.Addrs) == 0 || result.RTT == 0 {
            continue
        }
        peers = append(peers, PeerInfo{RTT: result.RTT, ID: p.ID, Addrs: p.Addrs})
	}

    sort.Slice(peers, func(i, j int) bool {
        return peers[i].RTT < peers[j].RTT
    })

    return peers
}


type LCAHost struct {
    Ctx                context.Context
    Host               host.Host
    DHT                *dht.IpfsDHT
	RoutingDiscovery   *discovery.RoutingDiscovery
}

func New(ctx context.Context, listenAddresses []string, streamHandler func(stream network.Stream), handlerProtocolID protocol.ID, rendezvous string) (LCAHost, error) {
    var err error

    // Populate gobal node variable
    var node LCAHost

    node.Ctx = ctx

    if len(listenAddresses) != 0 {
        node.Host, err = libp2p.New(node.Ctx,
            libp2p.ListenAddrStrings(listenAddresses...),
        )
        if err != nil {
            return node, err
        }
    } else {
        node.Host, err = libp2p.New(node.Ctx,
            libp2p.ListenAddrs(DefaultListenAddresses...),
        )
    }
    if err != nil {
        return node, err
    }

    if streamHandler != nil {
        node.Host.SetStreamHandler(handlerProtocolID, streamHandler)
    }

    node.DHT, err = dht.New(node.Ctx, node.Host)
    if err != nil {
        return node, err
    }

    // Bootstrap DHT
    bootstrapPeers := DefaultBootstrapPeers
    var wg sync.WaitGroup
    for _, peerAddr := range bootstrapPeers {
        peerinfo, _ := peer.AddrInfoFromP2pAddr(peerAddr)
        wg.Add(1)
        go func() {
            defer wg.Done()
            if err := node.Host.Connect(node.Ctx, *peerinfo); err != nil {
                fmt.Println(err)
            } else {
                fmt.Println("Connection established with bootstrap node:", *peerinfo)
            }
        }()
    }
    wg.Wait()

    if err = node.DHT.Bootstrap(node.Ctx); err != nil {
        return node, err
    }

    // Register new RoutingDiscovery
    node.RoutingDiscovery = discovery.NewRoutingDiscovery(node.DHT)
    discovery.Advertise(node.Ctx, node.RoutingDiscovery, rendezvous)

    return node, nil
}
