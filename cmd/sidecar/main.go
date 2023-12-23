package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Orlion/hersql/config"
	"github.com/Orlion/hersql/log"
	"github.com/Orlion/hersql/sidecar"
)

var configFile *string = flag.String("conf", "", "hersql sidecar config file")

func main() {
	flag.Parse()
	conf, err := config.ParseSidecarConfig(*configFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "configuration file parse error: "+err.Error())
		os.Exit(1)
	}

	log.Init(conf.Log)
	//需要退出的服务器
	serverOff := make(chan *sidecar.Server, len(conf.Servers))
	//已退出的服务器，用map存储
	serverOffMap := make(map[string]bool, len(conf.Servers))
	//配置map循环
	for name, serverConf := range conf.Servers {
		srv, err := sidecar.NewServer(serverConf)
		if err != nil {
			fmt.Fprintln(os.Stderr, "server error: "+err.Error())
			os.Exit(1)
		}
		go func() {
			if err = srv.ListenAndServe(); err != nil {
				fmt.Fprintln(os.Stderr, "server error: "+err.Error())
				serverOff <- srv
				serverOffMap[name] = true
			}
		}()
	}
	//serverOff 通道中有值，说明有服务器退出
	go func() {
		for {
			select {
			case srv := <-serverOff:
				fmt.Println("serverOff:", srv)
				ctx, _ := context.WithTimeout(context.Background(), 3000*time.Millisecond)
				srv.Shutdown(ctx)
			}
		}
	}()
	//如果所有服务器都退出了，那么退出程序
	for {
		if len(serverOffMap) == len(conf.Servers) {
			fmt.Println("all server off")
			log.Shutdown()
			os.Exit(1)
		}
		time.Sleep(1 * time.Second)
	}
}

func waitGracefulStop(srv *sidecar.Server) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, syscall.SIGHUP, syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT)
	for {
		s := <-c
		switch s {
		case syscall.SIGQUIT, syscall.SIGTERM, syscall.SIGINT:
			log.Infow("received stop signal", "signal", s.String())
			ctx, _ := context.WithTimeout(context.Background(), 3000*time.Millisecond)
			srv.Shutdown(ctx)
			log.Shutdown()
			time.Sleep(500 * time.Millisecond)
			return
		case syscall.SIGHUP:
		default:
		}
	}
}
