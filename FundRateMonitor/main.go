package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gorilla/websocket"
	"io"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const (
	symbol          = "ethusdt"     // 交易对符号
	mobile          = "139xxxxxxxx" // 预警接收手机号
	rateThreshold   = 5             // 费率阈值，低于这个百分比就短信预警，利润太低
	liquidThreshold = 5000          // 爆仓阈值，币价高于此值就短信预警，追加保证金
	closingThread   = 3200          // 平空阈值，币价低于此值就短信预警，波段获利了结
	smsInterval     = 600           // 预警短信重发间隔时间10分钟
)

type MarkPriceUpdate struct { // 交易所资金费率结构
	EventType        string `json:"e"` // 事件类型
	EventTime        int64  `json:"E"` // 事件时间（Unix 时间戳）
	Symbol           string `json:"s"` // 交易对
	MarkPrice        string `json:"p"` // 标记价格
	EstimatedFunding string `json:"r"` // 实时资金费率
	SettlementTime   int64  `json:"T"` // 下一次结算时间（Unix 时间戳）
}

var msg string
var msgMutex sync.RWMutex

func main() {
	go startWebServer()
	processCEX()
}

// startWebServer 开启 WebServer
func startWebServer() error {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		result := getMessage()
		if result == "" {
			result = "no data，waiting..."
		}
		content := `
<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>` + result + `</title>
    <meta http-equiv="refresh" content="10">
</head>
<body>
    <p>` + result + `</p>
</body>
</html>`
		fmt.Fprintf(w, content)
	})

	// 启动 Web 服务
	port := 8081
	fmt.Println("Server is running at", port)
	err := http.ListenAndServe(":"+strconv.Itoa(port), nil)
	if err != nil {
		log.Println("Error starting server:", err)
	}
	return err
}

// processCEX 处理交易所数据
func processCEX() {
	// 币安 websocket api
	wsURL := fmt.Sprintf("wss://fstream.binance.com/ws/%s@markPrice", symbol)
	for {
		time.Sleep(5 * time.Second)

		// 连接 WebSocket
		conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
		if err != nil {
			log.Println("WebSocket Connection Error:", err)
			continue
		}

		smsSentTime := int64(0) // 短信发送计时
		for {
			// 读取交易所数据
			_, message, err := conn.ReadMessage()
			if err != nil {
				log.Println("连接无法读取数据:", err)
				break
			}

			// 解析 JSON 数据
			var update MarkPriceUpdate
			err = json.Unmarshal(message, &update)
			if err != nil {
				fmt.Println("JSON 解析错误:", err)
				break
			}

			// 提取资金费率
			rValue := update.EstimatedFunding
			var rFloat float64
			_, err = fmt.Sscanf(rValue, "%f", &rFloat)
			if err != nil {
				log.Println("提取资金费率错误:", err)
				break
			}
			rPercentage := rFloat * 100
			apyPercentage := rFloat * 100 * 6 * 365

			// 提取标价
			var pMarketPrice float64
			_, err = fmt.Sscanf(update.MarkPrice, "%f", &pMarketPrice)
			if err != nil {
				log.Println("提取标价错误:", err)
				break
			}
			eventTime := time.UnixMilli(update.EventTime).Format("15:04:05")

			// 输出结果
			result := fmt.Sprintf("%s  年化:%.0f  标价:%.2f  费率:%.4f  符号:%s\n",
				eventTime, apyPercentage, pMarketPrice, rPercentage, symbol)
			setMessage(result)
			fmt.Printf(result)

			currentTime := time.Now().Unix()
			if currentTime-smsSentTime > smsInterval {
				smsSentTime = currentTime
				smsTemplate := "[年化:%.0f%%，价格：%.2f，%s]"
				if pMarketPrice > liquidThreshold {
					// 可能爆仓，需要补充保证金
					content := fmt.Sprintf(smsTemplate, apyPercentage, pMarketPrice, "补")
					sendSms(mobile, content)
				} else if apyPercentage < rateThreshold {
					// 负利率，自行决定是否平仓
					content := fmt.Sprintf(smsTemplate, apyPercentage, pMarketPrice, "负")
					sendSms(mobile, content)
				} else if pMarketPrice < closingThread {
					// 可以收割波段，自行决定是否平仓
					content := fmt.Sprintf(smsTemplate, apyPercentage, pMarketPrice, "平")
					sendSms(mobile, content)
				}
			}
		}

		_ = conn.Close()
	}
}

// setMessage 设置消息内容
func setMessage(newMsg string) {
	msgMutex.Lock() // 写操作加写锁
	defer msgMutex.Unlock()
	msg = newMsg
}

// getMessage 获取消息内容
func getMessage() string {
	msgMutex.RLock() // 读操作加读锁
	defer msgMutex.RUnlock()
	return msg
}

// sendSms 发送短信接口。mobile：手机号, content：短信内容
func sendSms(mobile, content string) error {
	formData := url.Values{}
	formData.Set("appid", "xxxxx")           // 自行购买填入
	formData.Set("signature", "xxxxxxxxxxx") // 自行购买填入
	formData.Set("timestamp", strconv.Itoa(int(time.Now().Unix())))
	formData.Set("to", mobile)
	formData.Set("content", content)
	requestBody := bytes.NewBufferString(formData.Encode())

	urlSms := "https://api.mysubmail.com/message/send" // 短信网关，自己解决
	contentType := "application/x-www-form-urlencoded"
	response, err := http.Post(urlSms, contentType, requestBody)
	if err != nil {
		log.Println("短信网关调用错误:", err)
		return err
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Println(err)
		}
	}(response.Body)

	var responseBody []byte
	_, err = response.Body.Read(responseBody)
	if err != nil {
		log.Println("短信网关响应错误:", err)
		return err
	}
	if response.StatusCode != http.StatusOK {
		log.Println("发送短信错误码:", response.StatusCode)
		return errors.New("sms error")
	}
	return nil
}
