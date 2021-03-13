package server

import (
	"golang.org/x/net/websocket"
	"net/http"
	"os"
	"strings"
	"time"
	"xSocks/param"
)



func StartWebSocket(addr string) error {
	http.HandleFunc("/",webHandler)
	if param.KeyFile==""||param.CertFile=="" {
		param.KeyFile="localhost_server.key"
		param.CertFile="localhost_server.pem"
		addrs:=strings.Split(addr,":")
		var ip="127.0.0.1";
		if(addrs[0]!="0.0.0.0"||addrs[0]!=""){
			 ip=addrs[0];
		}
		_,err:=os.Stat(param.KeyFile)
		if err!=nil {
			genCERT("improvement","localhost",ip);
		}
	}

	err :=http.ListenAndServeTLS(addr,param.CertFile,param.KeyFile,nil)

	if err != nil {
		panic("ListenAndServe: " + err.Error())
	}
	return nil;
}


func webHandler(w http.ResponseWriter, req *http.Request){
	if req.Header.Get("token")!=param.Password {
		msg:="Current server time:"+time.Now().Format("2006-01-02 15:04:05");
		w.Header().Add("Connection","Close")
		w.Header().Add("Content-Type","text/html")
		w.Write([]byte(msg))
		return
	}
	websocket:=websocket.Handler(wsToStream);
	websocket.ServeHTTP(w,req);
}



/* wsToStream*/
func wsToStream(ws *websocket.Conn) {
	streamToSocks5Yamux(ws)
}




