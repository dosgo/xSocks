// +build windows

package comm

import (
	"errors"
	"fmt"
	"github.com/StackExchange/wmi"
	routetable "github.com/yijunjun/route-table"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"log"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"
	"unsafe"
)


func GetGateway()string {
	table, err := routetable.NewRouteTable()
	if err != nil {
		panic(err.Error())
	}
	defer table.Close()
	rows, err := table.Routes()
	if err != nil {
		panic(err.Error())
	}
	var minMetric uint32=0;
	var gwIp="";
	for _, row := range rows {
		if routetable.Inet_ntoa(row.ForwardDest, false)=="0.0.0.0" {
			if minMetric==0 {
				minMetric=row.ForwardMetric1;
				gwIp=routetable.Inet_ntoa(row.ForwardNextHop, false)
			}else{
				if row.ForwardMetric1<minMetric {
					minMetric=row.ForwardMetric1;
					gwIp=routetable.Inet_ntoa(row.ForwardNextHop, false)
				}
			}
		}
	}
	return gwIp;
}

func getAdapterList() (*syscall.IpAdapterInfo, error) {
	b := make([]byte, 1000)
	l := uint32(len(b))
	a := (*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0]))
	err := syscall.GetAdaptersInfo(a, &l)
	if err == syscall.ERROR_BUFFER_OVERFLOW {
		b = make([]byte, l)
		a = (*syscall.IpAdapterInfo)(unsafe.Pointer(&b[0]))
		err = syscall.GetAdaptersInfo(a, &l)
	}
	if err != nil {
		return nil, os.NewSyscallError("GetAdaptersInfo", err)
	}
	return a, nil
}

func NotifyIpChange(notifyCh chan int) error{
	var  notifyAddrChange        *syscall.Proc
	if iphlpapi, err := syscall.LoadDLL("Iphlpapi.dll"); err == nil {
		if p, err := iphlpapi.FindProc("NotifyAddrChange"); err == nil {
			notifyAddrChange = p
		}
	}
	if notifyAddrChange==nil {
		return errors.New("NotifyAddrChange\r\n");
	}
	for {
		notifyAddrChange.Call(0, 0)
		notifyCh <- 0
	}
}
func GetLocalAddresses() ([]lAddr ,error) {
	lAddrs := []lAddr{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil,err
	}

	aList, err := getAdapterList()
	if err != nil {
		return nil,err
	}


	for _, ifi := range ifaces {
		for ai := aList; ai != nil; ai = ai.Next {
			index := ai.Index
			if ifi.Index == int(index) {
				ipl := &ai.IpAddressList
				gwl := &ai.GatewayList
				for ; ipl != nil; ipl = ipl.Next  {
					itemAddr := lAddr{}
					itemAddr.Name=ifi.Name
					itemAddr.IpAddress=fmt.Sprintf("%s",ipl.IpAddress.String)
					itemAddr.IpMask=fmt.Sprintf("%s",ipl.IpMask.String)
					itemAddr.GateWay=fmt.Sprintf("%s",gwl.IpAddress.String)
					lAddrs=append(lAddrs,itemAddr)
				}
			}
		}
	}
	return lAddrs,err
}




//dns

const (
	DnsConfigDnsServerList int32 = 6
)

type char byte
type IpAddressString struct {
	DNS [4 * 10]char
}

type Ip4Array struct {
	AddrCount  uint32
	Ip4Address [1]IpAddressString
}

func GetDnsServer() []string {
	dns := []string{}
	dnsapi := windows.NewLazyDLL("Dnsapi.dll")
	dnsQuery := dnsapi.NewProc("DnsQueryConfig")
	bufferBytes := make([]byte, 60)
loop:
	buffer := (*Ip4Array)(unsafe.Pointer(&bufferBytes[0]))
	blen := len(bufferBytes)
	r1, _, _ := dnsQuery.Call(uintptr(DnsConfigDnsServerList), uintptr(0), uintptr(0), uintptr(0), uintptr(unsafe.Pointer(&bufferBytes[0])), uintptr(unsafe.Pointer(&blen)))
	if r1 == 234 {
		bufferBytes = make([]byte, blen)
		goto loop
	} else if r1 == 0 {

	} else {
		return dns
	}
	for i := uint32(1); i <= buffer.AddrCount; i++ {
		right := i * 4
		left := right - 4
		tmpChars := buffer.Ip4Address[0].DNS[left:right]
		tmpStr := []string{}
		for j := 0; j < len(tmpChars); j++ {
			tmpStr = append(tmpStr, fmt.Sprint(tmpChars[j]))
		}
		tmpDNS := strings.Join(tmpStr, ".")
		pDns := net.ParseIP(tmpDNS)
		if pDns == nil {
			continue
		}
		if !pDns.IsGlobalUnicast() {
			continue
		}
		dns = append(dns, tmpDNS)
	}
	return dns
}

func SetDNSServer(gwIp string,ip string,ipv6 string){
	log.Printf("SetDNSServer-gwIp:%s\r\n",gwIp)
	oldDns,dHCPEnabled,isIPv6:=GetDnsServerByGateWay(gwIp);
	lAdds,err:=GetLocalAddresses();
	var iName="";
	if err==nil {
		for _, v := range lAdds {
			if strings.Index(v.GateWay,gwIp)!=-1 {
				iName=v.Name;
				break;
			}
		}
	}

	ch := make(chan os.Signal, 1)
	signal.Notify(ch,
		syscall.SIGHUP,
		syscall.SIGINT,
		syscall.SIGTERM,
		syscall.SIGKILL,
		syscall.SIGABRT,
		syscall.SIGSEGV,
		syscall.SIGQUIT)
	go func() {
		_= <-ch
		if len(oldDns)>0 {
			resetDns(iName,"ip",dHCPEnabled,oldDns);
			if isIPv6 {
				resetDns(iName, "ipv6", dHCPEnabled, []string{ipv6});
				Ipv6Switch(true);
			}
		}
		os.Exit(0);
	}()
	//ipv4
	changeDns(iName,"ip",ip,oldDns)
	//ipv6
	if isIPv6 {
		changeDns(iName, "ipv6", ipv6, []string{ipv6})
	}

	//ipv4优先
	if isIPv6 {
		Ipv6Switch(false);
		defer Ipv6Switch(true);
	}
	exec.Command("ipconfig", "/flushdns").Run()
	/*
	if len(oldDns)>0 {
		defer resetDns(iName,"ip",dHCPEnabled,oldDns);
		if isIPv6 {
			defer resetDns(iName, "ipv6", dHCPEnabled, []string{ipv6});
		}
	}
	c := make(chan int)
	<-c*/
}


func WatchNotifyIpChange(){
	notifyCh := make(chan int)
	go NotifyIpChange(notifyCh)
	go func() {
		for _ = range notifyCh {
			time.Sleep(time.Second*5)
			gwIp:=GetGateway()
			fmt.Printf("SetDNSServer gwip:%s\r\n",gwIp)
			SetDNSServer(gwIp,"127.0.0.1","0:0:0:0:0:0:0:1");
		}
	}()
}


func changeDns(iName string,netType string,ip string,oldDns []string){
//	netsh interface ipv6 add dns
	//netsh interface ip set dnsservers xx static 127.0.0.1 192.168.9.102
	exec.Command("netsh", "interface",netType,"set","dnsservers",iName,"static",ip).Output()
	for _,v:=range oldDns{
		exec.Command("netsh", "interface",netType,"add","dnsservers",iName,v).Output()
	}
}

func resetDns(iName string,netType string,dHCPEnabled bool,oldDns []string){
	//dhcp
	if dHCPEnabled {
		exec.Command("netsh", "interface",netType,"set","dnsservers",iName,"dhcp").Output()
	}else {
		for i,v:=range oldDns{
			if i==0 {
				exec.Command("netsh", "interface", netType, "set", "dnsservers", iName, "static", oldDns[0]).Output()
			}else {
				exec.Command("netsh", "interface", netType, "add", "dnsservers", iName, v).Output()
			}
		}
	}
}


func GetDnsServerByGateWay(gwIp string)([]string,bool,bool){
	//DNSServerSearchOrder
	adapters,err:=GetNetworkAdapter()
	var isIpv6=false;
	if err!=nil {
		return nil,false,isIpv6;
	}
	for _,v:=range adapters{
		if len(v.DefaultIPGateway)>0&&v.DefaultIPGateway[0]==gwIp {
			for _,v2:=range v.IPAddress{
				if len(v2)>16{
					isIpv6=true;
					break;
				}
			}

			return v.DNSServerSearchOrder,v.DHCPEnabled,isIpv6;
		}
	}
	return nil,false,isIpv6;
}

type Network struct {
	Name       string
	IP         string
	MACAddress string
}

type intfInfo struct {
	Name       string
	MacAddress string
	Ipv4       []string
}

func GetNetworkInfo() error {
	intf, err := net.Interfaces()
	if err != nil {
		log.Fatal("get network info failed: %v", err)
		return err
	}
	var is = make([]intfInfo, len(intf))
	for i, v := range intf {
		ips, err := v.Addrs()
		if err != nil {
			log.Fatal("get network addr failed: %v", err)
			return err
		}
		//此处过滤loopback（本地回环）和isatap（isatap隧道）
		if !strings.Contains(v.Name, "Loopback") && !strings.Contains(v.Name, "isatap") {
			var network Network
			is[i].Name = v.Name
			is[i].MacAddress = v.HardwareAddr.String()
			for _, ip := range ips {
				if strings.Contains(ip.String(), ".") {
					is[i].Ipv4 = append(is[i].Ipv4, ip.String())
				}
			}
			network.Name = is[i].Name
			network.MACAddress = is[i].MacAddress
			if len(is[i].Ipv4) > 0 {
				network.IP = is[i].Ipv4[0]
			}

			fmt.Printf("network:=", network)
		}

	}

	return nil
}
//BIOS信息
func GetBiosInfo() string {
	var s = []struct {
		Name string
	}{}
	err := wmi.Query("SELECT Name FROM Win32_BIOS WHERE (Name IS NOT NULL)", &s) // WHERE (BIOSVersion IS NOT NULL)
	if err != nil {
		return ""
	}
	return s[0].Name
}
type NetworkAdapter struct {
	DNSServerSearchOrder   []string
	DefaultIPGateway []string
	IPAddress []string
	Caption    string
	DHCPEnabled  bool
	ServiceName  string
	IPSubnet   []string
	SettingID string
}


func GetNetworkAdapter() ([]NetworkAdapter,error){
	var s = []NetworkAdapter{}
	err := wmi.Query("SELECT Caption,SettingID,DNSServerSearchOrder,DefaultIPGateway,ServiceName,IPAddress,IPSubnet,DHCPEnabled       FROM Win32_NetworkAdapterConfiguration WHERE IPEnabled=True", &s) // WHERE (BIOSVersion IS NOT NULL)
	if err != nil {
		log.Printf("err:%v\r\n",err)
		return nil,err
	}
	return s,nil;
}


func AddRoute(tunNet string,tunGw string, tunMask string) error {
	cmd:=exec.Command("route", "add",tunNet,"mask",tunMask,tunGw,"metric","6")
	cmd.Run();
	fmt.Printf("cmd:%s\r\n",strings.Join(cmd.Args," "))
	exec.Command("ipconfig", "/flushdns").Run()
	return nil;
}

func Ipv6Switch(open bool)error{
	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, "SYSTEM\\CurrentControlSet\\Services\\TCPIP6\\Parameters", registry.ALL_ACCESS)
	if err != nil {
		return err
	}
	defer key.Close()
	if open {
		key.SetDWordValue("DisabledComponents", 0x00)
	}else{
		key.SetDWordValue("DisabledComponents", 0x00000020)
	}
	return nil;
}