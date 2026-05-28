package http

import (
	"bytes"
	"common/utils"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
)

// TODO go client 改造,支持代理IP模式，看看有没有办法不要每次new client
// TODO 如果IP为空，则不使用代理IP

var client = Init()

func Init() *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
	client := &http.Client{
		Transport: transport,
	}
	return client
}

func InitHttpClient(ip string) *http.Client {
	if ip == "" {
		return client
	}
	var transport = &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     20 * time.Second,
	}
	transport.Proxy = http.ProxyURL(&url.URL{Host: ip})
	// transport.Proxy = http.ProxyURL(&url.URL{Host: "127.0.0.1:8888"})

	client := &http.Client{
		Transport: transport,
	}
	return client
}
func Get(requestUrl string, cookie string, headers map[string]string, ip string) (map[string]interface{}, error) {
	// 发送GET请求
	request, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		logrus.Errorf("请求失败: %v", err)
	}
	if cookie != "" {
		request.Header.Set("cookie", cookie)
	}
	if headers != nil {
		for key, value := range headers {
			request.Header.Set(key, value)
		}
	}
	result := map[string]interface{}{}
	response, err := InitHttpClient(ip).Do(request)
	if err != nil {
		logrus.Errorf("Error making GET request: %v", err)
		return result, err
	}
	defer response.Body.Close()

	// Read and print the response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		logrus.Errorf("Error reading response body: %v", err)
	}
	json.Unmarshal(body, &result)
	return result, err

}

func GetToResponse(requestUrl string, cookie string, headers map[string]string, ip string) (*http.Response, error) {
	// 发送GET请求
	request, err := http.NewRequest("GET", requestUrl, nil)
	if err != nil {
		logrus.Errorf("请求失败: %v", err)
	}
	if cookie != "" {
		request.Header.Set("cookie", cookie)
	}
	if headers != nil {
		for key, value := range headers {
			request.Header.Set(key, value)
		}
	}
	response, err := InitHttpClient(ip).Do(request)
	if err != nil {
		logrus.Errorf("GetToResponse Error making POST request: %v", err)
		return nil, err
	}
	return response, err
}

func PostForm(requestUrl string, requestBody map[string]interface{}, cookie string, headers map[string]string, ip string) (map[string]interface{}, error) {
	// Encode the struct to JSON
	formData := url.Values{}
	for key, value := range requestBody {
		formData.Add(key, utils.InterfaceToString(value))
	}
	request, err := http.NewRequest("POST", requestUrl, strings.NewReader(formData.Encode()))
	if err != nil {
		logrus.Errorf("[ERROR] Error creating request: %v", err)
		return nil, err
	}
	// Set the appropriate headers
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	if headers != nil {
		for key, value := range headers {
			request.Header.Set(key, value)
		}
	}
	result := map[string]interface{}{}
	response, err := InitHttpClient(ip).Do(request)
	if err != nil {
		logrus.Errorf("[ERROR] Error making PostForm request: %v", err)
		return result, err
	}
	defer response.Body.Close()

	// Read and print the response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		logrus.Errorf("[ERROR] Error reading response body: %v", err)
		return result, err
	}
	json.Unmarshal(body, &result)
	return result, err
}

func Post(requestUrl string, requestBody map[string]interface{}, cookie string, headers map[string]string, ip string) (map[string]interface{}, error) {
	// Encode the struct to JSON
	jsonData, _ := json.Marshal(requestBody)
	request, err := http.NewRequest("POST", requestUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		logrus.Errorf("[ERROR] Error creating request: %v", err)
		return nil, err
	}
	// Set the appropriate headers
	request.Header.Set("Content-Type", "application/json")
	if cookie != "" {
		request.Header.Set("Cookie", cookie)
	}
	if headers != nil {
		for key, value := range headers {
			request.Header.Set(key, value)
		}
	}
	result := map[string]interface{}{}
	response, err := InitHttpClient(ip).Do(request)
	if err != nil {
		logrus.Errorf("Error making POST request: %v", err)
		return result, err
	}
	defer response.Body.Close()

	// Read and print the response body
	body, err := io.ReadAll(response.Body)
	if err != nil {
		logrus.Errorf("[ERROR] Error reading response body: %v", err)
		return result, err
	}
	json.Unmarshal(body, &result)
	return result, err
}
