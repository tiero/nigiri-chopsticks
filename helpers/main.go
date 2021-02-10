package helpers

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

// RpcClient represents a JSON RPC client (over HTTP(s)).
type RpcClient struct {
	serverAddr string
	httpClient *http.Client
	timeout    int
}

// rpcRequest represent a RCP request
type rpcRequest struct {
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	Id      int64       `json:"id"`
	JsonRpc string      `json:"jsonrpc"`
}

type rpcResponse struct {
	Id     int64           `json:"id"`
	Result json.RawMessage `json:"result"`
	Err    interface{}     `json:"error"`
}

func NewRpcClient(url string, useSSL bool, timeout int) (c *RpcClient, err error) {
	var httpClient *http.Client

	if useSSL {
		t := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		httpClient = &http.Client{Transport: t}
	} else {
		httpClient = &http.Client{}
	}

	c = &RpcClient{
		serverAddr: url,
		httpClient: httpClient,
		timeout:    timeout,
	}

	return
}

// Call prepare & exec the request
func (c *RpcClient) Call(method string, params interface{}) (status int, rr rpcResponse, err error) {
	status = http.StatusInternalServerError
	connectTimer := time.NewTimer(time.Duration(c.timeout) * time.Second)
	rpcR := rpcRequest{method, params, time.Now().UnixNano(), "1.0"}
	payloadBuffer := &bytes.Buffer{}
	if err = json.NewEncoder(payloadBuffer).Encode(rpcR); err != nil {
		return
	}

	req, err := http.NewRequest("POST", c.serverAddr, payloadBuffer)
	if err != nil {
		return
	}
	req.Header.Add("Content-Type", "application/json;charset=utf-8")
	req.Header.Add("Accept", "application/json")

	resp, err := c.doTimeoutRequest(connectTimer, req)
	if err != nil {
		return
	}
	defer resp.Body.Close()

	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return
	}
	if resp.StatusCode != http.StatusOK {
		out := map[string]map[string]interface{}{}
		json.Unmarshal(data, &out)
		err = fmt.Errorf("Method %s failed with error: %s", method, out["error"]["message"].(string))
		status = resp.StatusCode
		return
	}

	if err = json.Unmarshal(data, &rr); err != nil {
		return
	}

	status = resp.StatusCode
	return
}

// doTimeoutRequest process a HTTP request with timeout
func (c *RpcClient) doTimeoutRequest(timer *time.Timer, req *http.Request) (*http.Response, error) {
	type result struct {
		resp *http.Response
		err  error
	}
	done := make(chan result, 1)
	go func() {
		resp, err := c.httpClient.Do(req)
		done <- result{resp, err}
	}()
	// Wait for the read or the timeout
	select {
	case r := <-done:
		return r.resp, r.err
	case <-timer.C:
		return nil, fmt.Errorf("Timeout reading data from server")
	}
}

// HandleRPCRequest call a JSONRPC and decoded the JSON body as response
func HandleRPCRequest(client *RpcClient, method string, params []interface{}) (int, interface{}, error) {
	status, resp, err := client.Call(method, params)
	if err != nil {
		return status, "", err
	}
	var out interface{}
	err = json.Unmarshal(resp.Result, &out)
	if err != nil {
		return http.StatusInternalServerError, "", err
	}

	return status, out, nil
}
