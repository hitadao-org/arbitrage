package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	mobile      = "139xxxxxxxx" // 通知接收的手机号
	smsInterval = 600           // 通知短信重发间隔时间10分钟
)

type CoinData struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    []struct {
		CoinId        string `json:"CoinId"` //币代码
		StandardPrice string `json:"StandardPrice"`//币价格
	} `json:"data"`
}

func main() {
	processCoin()
}

// getCoinPrice 获取指定币的价格
func getCoinPrice(coinName string) (string, error) {
	// 发起 HTTP GET 请求
	url := "https://web3daily.news/news/api/tnv/coinData/list"
	resp, err := http.Get(url)
	if err != nil {
		log.Println("Error making GET request: ", err)
		return "", err
	}
	defer resp.Body.Close()

	// 解析 JSON 数据
	var coinData CoinData
	err = json.NewDecoder(resp.Body).Decode(&coinData)
	if err != nil {
		log.Println("Error decoding JSON response:", err)
	}

	for _, coin := range coinData.Data {
		if coin.CoinId == "BTCUSDT" && coinName == "btc" {
			return coin.StandardPrice, nil
		}
		if coin.CoinId == "ETHUSDT" && coinName == "eth" {
			return coin.StandardPrice, nil
		}
		if coin.CoinId == "SOLUSDT" && coinName == "sol" {
			return coin.StandardPrice, nil
		}
	}
	return "", errors.New("no data")
}

// processCoin 处理交易所币价数据
func processCoin() {
	// os.Args[0] 是程序的名字，os.Args[1:] 是传入的命令行参数
	if len(os.Args) < 4 {
		log.Fatal("Please provide an integer argument")
		return
	}

	// 币种
	coinName := strings.ToLower(os.Args[1])
	if coinName != "btc" && coinName != "eth" && coinName != "sol" {
		log.Fatal("Invalid coin name")
		return
	}
	log.Println("coinName:", coinName)

	// 价格类型
	alertType := strings.ToLower(os.Args[2])
	if alertType != "lt" && alertType != "gt" {
		log.Fatal("Invalid price type")
		return
	}
	log.Println("alertType:", alertType)

	// 价格阈值
	arg := os.Args[3]
	alertPrice, err := strconv.ParseFloat(arg, 15)
	if err != nil {
		log.Fatal("alertPrice:", err)
		return
	}
	log.Println("threshold:", alertPrice)

	smsSentTime := int64(0)

	for {
		time.Sleep(10 * time.Second)
		sPrice, err := getCoinPrice(coinName)
		if err != nil {
			continue
		}
		log.Println(sPrice)
		price, err := strconv.ParseFloat(sPrice, 15)
		if err != nil {
			continue
		}
		currentTime := time.Now().Unix()
		if currentTime-smsSentTime > int64(smsInterval) {
			smsSentTime = currentTime
			smsTemplate := "[%s：%.2f] "
			coinName := ""
			if coinName == "btc" {
				coinName = "大饼"
			} else if coinName == "eth" {
				coinName = "二饼"
			} else if coinName == "sol" {
				coinName = "三饼"
			}
			if alertType == "lt" && price < alertPrice {
				log.Println(coinName, "lt threshold")
				content := fmt.Sprintf(smsTemplate, coinName, price)
				sendSms(mobile, content)
			} else if alertType == "gt" && price > alertPrice {
				log.Println(coinName, "gt threshold")
				content := fmt.Sprintf(smsTemplate, coinName, price)
				sendSms(mobile, content)
			}
		}
	}
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