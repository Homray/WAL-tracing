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

type idResp struct {
	ID int `json:"id"`
}

type linkKey struct {
	oid int
	eid int
}

func main() {
	orderURL := mustEnv("ORDER_URL")
	execURL := mustEnv("EXECUTOR_URL")
	logiURL := mustEnv("LOGISTICS_URL")

	time.Sleep(4 * time.Second)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	client := &http.Client{Timeout: 2 * time.Second}

	var orderIDs []int
	var execIDs []int
	var links []linkKey

	ensureOrder := func() (int, bool) {
		if len(orderIDs) > 0 {
			return orderIDs[rng.Intn(len(orderIDs))], true
		}
		resp, err := post(client, orderURL+"/create")
		if err != nil || resp == nil {
			return 0, false
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, false
		}
		var r idResp
		if json.Unmarshal(body, &r) != nil || r.ID <= 0 {
			return 0, false
		}
		orderIDs = append(orderIDs, r.ID)
		return r.ID, true
	}

	ensureExec := func() (int, bool) {
		if len(execIDs) > 0 {
			return execIDs[rng.Intn(len(execIDs))], true
		}
		resp, err := post(client, execURL+"/create")
		if err != nil || resp == nil {
			return 0, false
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, false
		}
		var r idResp
		if json.Unmarshal(body, &r) != nil || r.ID <= 0 {
			return 0, false
		}
		execIDs = append(execIDs, r.ID)
		return r.ID, true
	}

	log.Println("loadgen-logistics started")
	log.Println(" order-service    =", orderURL)
	log.Println(" executor-service =", execURL)
	log.Println(" logistics-service=", logiURL)

	for {
		x := rng.Intn(100)

		switch {
		case x < 65: // LINK
			oid, ok1 := ensureOrder()
			eid, ok2 := ensureExec()
			if !ok1 || !ok2 {
				break
			}
			_, _ = post(
				client,
				logiURL+"/link?order_id="+strconv.Itoa(oid)+"&executor_id="+strconv.Itoa(eid),
			)
			links = append(links, linkKey{oid: oid, eid: eid})

		default: // UNLINK
			if len(links) == 0 {
				break
			}
			idx := rng.Intn(len(links))
			k := links[idx]
			_, _ = post(
				client,
				logiURL+"/unlink?order_id="+strconv.Itoa(k.oid)+"&executor_id="+strconv.Itoa(k.eid),
			)

			links[idx] = links[len(links)-1]
			links = links[:len(links)-1]
		}

		time.Sleep(time.Duration(20+rng.Intn(120)) * time.Millisecond)
	}
}
