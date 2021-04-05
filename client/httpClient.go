package client

import (
	"crypto/tls"
	"net/http"
	"net/url"
	"sync"
	"xSocks/client/httpcomm"
	"xSocks/comm"
	"xSocks/param"
)

/*
http1.1
*/

type httpConn struct {
	sync.Mutex
	client *http.Client;
}

var httpDialer *httpConn
func init(){

	httpDialer=&httpConn{}
}


func NewHttpDialer()  *httpConn{
	httpDialer.client=newHttpClient()
	return httpDialer;
}

func newHttpClient() *http.Client{
	tslClientConf:=httpcomm.GetTlsConf();
	t := &http.Transport{TLSClientConfig: tslClientConf}
	return  &http.Client{Transport: t}
}

func (qd *httpConn) Dial(_url string) (comm.CommConn, error) {
	qd.Lock()
	defer qd.Unlock()
	tslClientConf:=httpcomm.GetTlsConf();
	urlInfo, err := url.Parse(_url)
	conn, err := tls.Dial("tcp", urlInfo.Host, tslClientConf)

	if err!=nil {
		return nil,err;
	}


	header  := "CONNECT /"+urlInfo.Host+" HTTP/1.1\r\n";
	header += "Host:"+urlInfo.Host+"\r\n";
	header += "Proxy-Connection: Keep-Alive\r\n";
	header += "token: "+param.Password+"\r\n";
	header += "Content-Length: 0\r\n\r\n";

	conn.Write([]byte(header))

	buf:=make([]byte,1024);
	conn.Read(buf)
	return  conn,nil;
}
