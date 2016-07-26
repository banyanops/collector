// Copyright (C) 2010, Kyle Lemons <kyle@kylelemons.net>.  All rights reserved.

package log4go

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
)

// This log writer sends output to a socket
type SocketLogWriter struct {
	sock 	net.Conn
	proto	string
	hostport string
}

func (w *SocketLogWriter) Close() {
	if w.sock != nil {
		w.sock.Close()
	}
}

func NewSocketLogWriter(proto, hostport string) *SocketLogWriter {
	s := &SocketLogWriter{
		sock:	nil,
		proto:	proto,
		hostport:	hostport,
	}
	return s
}

func (s *SocketLogWriter) LogWrite(rec *LogRecord) {

	// Marshall into JSON
	js, err := json.Marshal(rec)
	if err != nil {
		fmt.Fprintf(os.Stderr, "SocketLogWriter(%s): %v\n", s.hostport, err)
		return
	}

	if s.sock == nil {
		s.sock, err = net.Dial(s.proto, s.hostport)
		if err != nil {
			fmt.Fprintf(os.Stderr, "SocketLogWriter(%s): %v\n", s.hostport, err)
			if s.sock != nil {
				s.sock.Close()
				s.sock = nil
			}
			return
		}
	}

	_, err = s.sock.Write(js)
	if err == nil {
		return
	}

	fmt.Fprintf(os.Stderr, "SocketLogWriter(%s): %v\n", s.hostport, err)
	s.sock.Close()
	s.sock = nil
}

