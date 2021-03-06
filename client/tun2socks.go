package client

import (
	"fmt"
	"github.com/dosgo/xsocks/client/tun"
	"github.com/dosgo/xsocks/client/tun2socks"
	"github.com/dosgo/xsocks/comm"
	"github.com/dosgo/xsocks/param"
	"gvisor.dev/gvisor/pkg/tcpip"
	"gvisor.dev/gvisor/pkg/tcpip/adapters/gonet"
	"io"
	"log"
	"net"
	"net/url"
	"runtime"
	"strings"
	"time"
)

type Tun2Socks struct {
	tunDev io.ReadWriteCloser
	remoteAddr string;
	dnsServers []string;
	oldGw string;
	tunGW string;
}


/*tunType==1*/
func (_tun2socks *Tun2Socks)Start(tunDevice string,tunAddr string,tunMask string,tunGW string,tunDNS string)  error{
	_tun2socks.oldGw=comm.GetGateway();
	_tun2socks.tunGW=tunGW;
	_tun2socks.dnsServers = strings.Split(tunDNS, ",")
	var err error;
	if len(param.Args.UnixSockTun)>0 {
		_tun2socks.tunDev,err= tun.UsocketToTun(param.Args.UnixSockTun)
		if err != nil {                      //如果监听失败，一般是文件已存在，需要删除它
			return err;
		}
	}else{
		if runtime.GOOS=="windows" {
			urlInfo, _ := url.Parse(param.Args.ServerAddr)
			addr, err := net.ResolveIPAddr("ip",urlInfo.Hostname())
			if err == nil {
				_tun2socks.remoteAddr = addr.String()
			}
			fmt.Printf("remoteAddr:%s\r\n", _tun2socks.remoteAddr)
		}

		_tun2socks.tunDev,err= tun.RegTunDev(tunDevice ,tunAddr ,tunMask ,tunGW ,tunDNS )
		if err != nil {
			fmt.Println("start tun err:", err)
			return err;
		}
	}

	//windows
	if runtime.GOOS=="windows" {
		oldDns:=comm.GetDnsServer();
		if oldDns!=nil&&len(oldDns)>0 {
			_tun2socks.dnsServers  = append(_tun2socks.dnsServers , oldDns...)
		}
		routeEdit(tunGW,_tun2socks.remoteAddr,_tun2socks.dnsServers ,_tun2socks.oldGw);
	}
	go tun2socks.ForwardTransportFromIo(_tun2socks.tunDev,param.Args.Mtu,rawTcpForwarder,rawUdpForwarder);
	return nil;
}
/**/
func (_tun2socks *Tun2Socks) Shutdown(){
	if _tun2socks.tunDev!=nil {
		_tun2socks.tunDev.Close();
	}
	unRegRoute(_tun2socks.tunGW ,_tun2socks.remoteAddr ,_tun2socks.dnsServers ,_tun2socks.oldGw)
}


func rawTcpForwarder(conn *gonet.TCPConn)error{
	var remoteAddr=conn.LocalAddr().String()
	//dns ,use 8.8.8.8
	if strings.HasSuffix(remoteAddr,":53") {
		dnsReqTcp(conn);
		return  nil;
	}
	socksConn,err1:= net.DialTimeout("tcp",param.Args.Sock5Addr,time.Second*15)
	if err1 != nil {
		log.Printf("err:%v",err1)
		return nil
	}
	defer socksConn.Close();
	if tun2socks.SocksCmd(socksConn,1,remoteAddr)==nil {
		comm.TcpPipe(conn,socksConn,time.Minute*5)
	}
	return nil
}

func rawUdpForwarder(conn *gonet.UDPConn, ep tcpip.Endpoint)error{
	defer ep.Close();
	defer conn.Close();
	//dns port
	if strings.HasSuffix(conn.LocalAddr().String(),":53") {
		dnsReqUdp(conn);
	}else{
		dstAddr,_:=net.ResolveUDPAddr("udp",conn.LocalAddr().String())
		tun2socks.SocksUdpGate(conn,dstAddr);
	}
	return nil;
}
func dnsReqUdp(conn *gonet.UDPConn) error{
	dnsConn, err := net.DialTimeout("udp", "127.0.0.1:"+param.Args.DnsPort,time.Second*15);
	if err != nil {
		fmt.Println(err.Error())
		return err;
	}
	comm.UdpPipe(conn,dnsConn,time.Minute*5)
	return nil;
}
/*to dns*/
func dnsReqTcp(conn *gonet.TCPConn) error{
	dnsConn, err := net.DialTimeout("tcp", "127.0.0.1:"+param.Args.DnsPort,time.Second*15);
	if err != nil {
		fmt.Println(err.Error())
		return err;
	}
	comm.TcpPipe(conn,dnsConn,time.Minute*2)
	fmt.Printf("dnsReq Tcp\r\n");
	return nil;
}




