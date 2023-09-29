package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/push"
)

const (
	Port = ":8083"

	PrometheusUrl = "http://127.0.0.1:9092"
	PrometheusJob = "gin_test_prometheus"

	PrometheusNamespace    = "gin_test_data"
	EndpointsDataSubsystem = "endpoints"
	ErrCodeDataSubsystem   = "code"
)

var (
	pusher *push.Pusher

	endpointsErrcodeMonitor = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Namespace: PrometheusNamespace,
			Subsystem: EndpointsDataSubsystem,
			Name:      "errcode_statistic",
			Help:      "统计接口错误码信息数据",
		}, []string{EndpointsDataSubsystem, ErrCodeDataSubsystem},
	)
)

var (
	SUCCESS         = NewRespCode(1000, "Success")
	ERROR_MYSQL     = NewRespCode(2000, "MySQL发生错误")
	ERROR_REDIS     = NewRespCode(2001, "Redis发生错误")
	ERRROR_INTERNAL = NewRespCode(2002, "Internal发生错误")
)

type DataResp struct {
	Code int
	Msg  string
	Data any
}

type RespCode struct {
	Code int
	Msg  string
}

func NewRespCode(code int, msg string) RespCode {
	return RespCode{
		Code: code,
		Msg:  msg,
	}
}

func NewDataResp(respCode RespCode, data any) DataResp {
	return DataResp{
		Code: respCode.Code,
		Msg:  respCode.Msg,
		Data: data,
	}
}

func init() {
	pusher = push.New(PrometheusUrl, PrometheusJob)
	prometheus.MustRegister(
		endpointsErrcodeMonitor,
	)
	pusher.Collector(endpointsErrcodeMonitor)
}

type Model struct {
	gin.ResponseWriter
	respBody *bytes.Buffer
}

func newModel(c *gin.Context) *Model {
	return &Model{
		c.Writer,
		bytes.NewBuffer([]byte{}),
	}
}

func (s Model) Write(b []byte) (int, error) {
	s.respBody.Write(b)
	return s.ResponseWriter.Write(b)
}

// 处理错误码的中间件会比较复杂，因为需要处理响应体的信息
// 所以需要通过改写gin的Context的方法来实现在中间件中获取错误体的信息
func HandleEndpointErrcode() gin.HandlerFunc {
	return func(c *gin.Context) {
		endpoint := c.Request.URL.Path

		model := newModel(c)
		// 改写gin.Context的Write 让响应体信息在我们的 model.respBody可查
		c.Writer = model
		defer func(c *gin.Context) {
			var resp DataResp
			fmt.Println(model.respBody.String())
			if err := json.Unmarshal(model.respBody.Bytes(), &resp); err != nil {
				log.Println("解析响应体失败: %+v", resp)
				panic(err)
			}

			endpointsErrcodeMonitor.With(prometheus.Labels{EndpointsDataSubsystem: endpoint, ErrCodeDataSubsystem: resp.Msg}).Inc()
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
			time.Sleep(1 * time.Second)
		}
	}()

	r.Use(HandleEndpointErrcode())
	var counter int

	r.GET("/hello", func(c *gin.Context) {
		counter++
		if counter%10 == 0 {
			c.JSON(http.StatusOK, NewDataResp(ERROR_MYSQL, "123"))
			return
		}

		if counter%2 == 1 {
			c.JSON(http.StatusOK, NewDataResp(SUCCESS, "123"))
			return
		}

		if counter%3 == 1 {
			c.JSON(http.StatusOK, NewDataResp(ERRROR_INTERNAL, "123"))
			return
		}
		if counter%3 == 2 {
			c.JSON(http.StatusOK, NewDataResp(ERROR_REDIS, "123"))
			return
		}
	})

	r.Run(Port)
}
