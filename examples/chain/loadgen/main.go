// loadgen generates a random stream of item create/update/delete requests
// directed at the api-gateway.
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
	client := &http.Client{Timeout: 3 * time.Second}

	categories := []string{"electronics", "books", "clothing", "food"}
	var ids []int

	log.Println("loadgen started, target =", target)

	for {
		x := rng.Intn(100)
		cat := categories[rng.Intn(len(categories))]

		switch {
		case x < 45: // CREATE
			url := target + "/items/create?name=item-" + strconv.Itoa(rng.Intn(1000)) + "&category=" + cat
			resp, err := post(client, url)
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

		case x < 85: // UPDATE
			if len(ids) > 0 {
				id := ids[rng.Intn(len(ids))]
				_, _ = post(client, target+"/items/update?id="+strconv.Itoa(id))
			}

		default: // DELETE
			if len(ids) > 0 {
				idx := rng.Intn(len(ids))
				id := ids[idx]
				_, _ = post(client, target+"/items/delete?id="+strconv.Itoa(id))
				ids[idx] = ids[len(ids)-1]
				ids = ids[:len(ids)-1]
			}
		}

		time.Sleep(time.Duration(15+rng.Intn(85)) * time.Millisecond)
	}
}
