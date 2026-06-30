// loadgen-executors generates a random mix of executor create/update/delete requests.
package main

import (
	"encoding/json"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"
)

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("%s is not set", key)
	}
	return v
}

func post(client *http.Client, url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	return client.Do(req)
}

func main() {
	target := mustEnv("TARGET_URL")
	time.Sleep(3 * time.Second)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	client := &http.Client{Timeout: 2 * time.Second}

	var ids []int
	log.Println("loadgen-executors started, target =", target)

	for {
		x := rng.Intn(100)
		switch {
		case x < 45: // CREATE
			resp, err := post(client, target+"/create")
			if err == nil && resp != nil {
				body, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					var res struct {
						ID int `json:"id"`
					}
					if json.Unmarshal(body, &res) == nil && res.ID > 0 {
						ids = append(ids, res.ID)
					}
				}
			}
		case x < 90: // UPDATE
			if len(ids) > 0 {
				id := ids[rng.Intn(len(ids))]
				_, _ = post(client, target+"/update?id="+strconv.Itoa(id))
			}
		default: // DELETE
			if len(ids) > 0 {
				idx := rng.Intn(len(ids))
				id := ids[idx]
				_, _ = post(client, target+"/delete?id="+strconv.Itoa(id))
				ids[idx] = ids[len(ids)-1]
				ids = ids[:len(ids)-1]
			}
		}
		time.Sleep(time.Duration(30+rng.Intn(120)) * time.Millisecond)
	}
}
