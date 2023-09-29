package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

const (
	Port = ":8082"

	PrometheusUrl = "http://127.0.0.1:9092"
	PrometheusJob = "gin_test_prometheus"

	PrometheusNamespace    = "gin_test_data"
	EndpointsDataSubsystem = "endpoints"
)

var (
	pusher *push.Pusher

	endpointsLantencyMonitor = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: PrometheusNamespace,
			Subsystem: EndpointsDataSubsystem,
			Name:      "lantency_statistic",
			Help:      "统计耗时数据",
			Buckets:   []float64{1, 5, 10, 20, 50, 100, 500, 1000, 5000, 10000},
		}, []string{EndpointsDataSubsystem},
	)
)

func init() {
	pusher = push.New(PrometheusUrl, PrometheusJob)
	prometheus.MustRegister(
		endpointsLantencyMonitor,
	)
	pusher.Collector(endpointsLantencyMonitor)
}

func HandleEndpointLantency() gin.HandlerFunc {
	return func(c *gin.Context) {
		endpoint := c.Request.URL.Path
		fmt.Println(endpoint)
		start := time.Now()
		defer func(c *gin.Context) {
			lantency := time.Now().Sub(start)
			lantencyStr := fmt.Sprintf("%0.3d", lantency.Nanoseconds()/1e6) // 记录ms数据，为小数点后3位
			lantencyFloat64, err := strconv.ParseFloat(lantencyStr, 64)     //转换成float64类型
			if err != nil {
				panic(err)
			}

			fmt.Println(lantencyFloat64)

			endpointsLantencyMonitor.With(prometheus.Labels{EndpointsDataSubsystem: endpoint}).Observe(lantencyFloat64)
		}(c)
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
		// 随机1秒内分钟访问一次接口
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

		for {
			req("/hello")
		}
	}()

	r.Use(HandleEndpointLantency())

	var count int
	r.GET("/hello", func(c *gin.Context) {
		count++

		if count%100 == 0 {
			suddenSecond := rand.Intn(10) // 0-10s
			time.Sleep(time.Duration(suddenSecond) * time.Second)
			c.JSON(http.StatusOK, gin.H{
				"Hello": "World",
			})
			return
		}

		normalSecond := rand.Intn(100) // 0-10ms

		time.Sleep(time.Duration(normalSecond) * time.Millisecond)

		c.JSON(http.StatusOK, gin.H{
			"Hello": "World",
		})
	})

	r.Run(Port)
}
