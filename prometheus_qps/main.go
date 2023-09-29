package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

const (
	Port = ":8082"

	PrometheusUrl = "http://127.0.0.1:9092"
	PrometheusJob = "gin_test_prometheus_qps"

	PrometheusNamespace    = "gin_test_data"
	EndpointsDataSubsystem = "endpoints"
)

var (
	pusher *push.Pusher

	endpointsQPSMonitor = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: PrometheusNamespace,
			Subsystem: EndpointsDataSubsystem,
			Name:      "QPS_statistic",
			Help:      "统计QPS数据",
		}, []string{EndpointsDataSubsystem},
	)
)

func init() {
	pusher = push.New(PrometheusUrl, PrometheusJob)
	prometheus.MustRegister(
		endpointsQPSMonitor,
	)
	pusher.Collector(endpointsQPSMonitor)
}

func HandleEndpointQps() gin.HandlerFunc {
	return func(c *gin.Context) {
		endpoint := c.Request.URL.Path
		fmt.Println(endpoint)
		// Counter .Add() 指标加1
		endpointsQPSMonitor.With(prometheus.Labels{EndpointsDataSubsystem: endpoint}).Inc()
		c.Next()
	}
}

func main() {
	r := gin.New()

	go func() {
		// 每15秒上报一次数据
		for range time.Tick(15 * time.Second) {
			if err := pusher.
				Add(); err != nil {
				log.Println(err)
			}
			log.Println("push ")
		}
	}()

	go func() {
		var req func(endpoint string)
		req = func(endpoint string) {
			defer func() {
				if r := recover(); r != nil {
					log.Println(r)
				}
			}()

			_, err := http.Get(fmt.Sprintf("http://localhost%s%s", Port, endpoint))
			if err != nil {
				panic(err)
			}
		}
		twoSecondTicker := time.NewTicker(time.Second * 2)
		halfSecondTicker := time.NewTicker(time.Second / 2)
		for {
			select {
			case <-halfSecondTicker.C:
				req("/world")
			case <-twoSecondTicker.C:
				req("/hello")
			}
		}
	}()

	r.Use(HandleEndpointQps())

	r.GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"Hello": "World",
		})
	})
	r.GET("/world", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"World": "Hello",
		})
	})

	r.Run(Port)
}
