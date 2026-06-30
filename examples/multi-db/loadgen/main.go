// loadgen — нагрузчик для примера multi-db.
// Отправляет запросы напрямую во все три сервиса в случайном порядке.
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

func readID(resp *http.Response) (int, bool) {
	if resp == nil {
		return 0, false
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return 0, false
	}
	var r idResp
	if json.Unmarshal(body, &r) != nil || r.ID <= 0 {
		return 0, false
	}
	return r.ID, true
}

func main() {
	userURL := mustEnv("USER_URL")
	productURL := mustEnv("PRODUCT_URL")
	reviewURL := mustEnv("REVIEW_URL")

	time.Sleep(4 * time.Second)

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	client := &http.Client{Timeout: 3 * time.Second}

	categories := []string{"electronics", "books", "clothing", "food", "sports"}

	var userIDs []int
	var productIDs []int
	var reviewIDs []int

	log.Println("loadgen (multi-db) started")
	log.Printf("  user-service    = %s", userURL)
	log.Printf("  product-service = %s", productURL)
	log.Printf("  review-service  = %s", reviewURL)

	for {
		x := rng.Intn(100)

		switch {
		case x < 20: // Создать пользователя
			n := rng.Intn(10000)
			resp, err := post(client, userURL+"/users/create?name=user"+strconv.Itoa(n)+"&email=user"+strconv.Itoa(n)+"@example.com")
			if err == nil {
				if id, ok := readID(resp); ok {
					userIDs = append(userIDs, id)
				}
			}

		case x < 35: // Обновить пользователя
			if len(userIDs) > 0 {
				id := userIDs[rng.Intn(len(userIDs))]
				_, _ = post(client, userURL+"/users/update?id="+strconv.Itoa(id))
			}

		case x < 40: // Удалить пользователя
			if len(userIDs) > 0 {
				idx := rng.Intn(len(userIDs))
				id := userIDs[idx]
				resp, _ := post(client, userURL+"/users/delete?id="+strconv.Itoa(id))
				if resp != nil && resp.StatusCode == http.StatusNoContent {
					resp.Body.Close()
					userIDs[idx] = userIDs[len(userIDs)-1]
					userIDs = userIDs[:len(userIDs)-1]
				} else if resp != nil {
					resp.Body.Close()
				}
			}

		case x < 55: // Создать товар
			n := rng.Intn(10000)
			cat := categories[rng.Intn(len(categories))]
			resp, err := post(client, productURL+"/products/create?name=product"+strconv.Itoa(n)+"&category="+cat)
			if err == nil {
				if id, ok := readID(resp); ok {
					productIDs = append(productIDs, id)
				}
			}

		case x < 70: // Обновить товар
			if len(productIDs) > 0 {
				id := productIDs[rng.Intn(len(productIDs))]
				_, _ = post(client, productURL+"/products/update?id="+strconv.Itoa(id))
			}

		case x < 75: // Удалить товар
			if len(productIDs) > 0 {
				idx := rng.Intn(len(productIDs))
				id := productIDs[idx]
				resp, _ := post(client, productURL+"/products/delete?id="+strconv.Itoa(id))
				if resp != nil && resp.StatusCode == http.StatusNoContent {
					resp.Body.Close()
					productIDs[idx] = productIDs[len(productIDs)-1]
					productIDs = productIDs[:len(productIDs)-1]
				} else if resp != nil {
					resp.Body.Close()
				}
			}

		case x < 92: // Создать отзыв (нужны и user, и product)
			if len(userIDs) > 0 && len(productIDs) > 0 {
				uid := userIDs[rng.Intn(len(userIDs))]
				pid := productIDs[rng.Intn(len(productIDs))]
				rating := rng.Intn(5) + 1
				resp, err := post(client, reviewURL+
					"/reviews/create?user_id="+strconv.Itoa(uid)+
					"&product_id="+strconv.Itoa(pid)+
					"&rating="+strconv.Itoa(rating))
				if err == nil {
					if id, ok := readID(resp); ok {
						reviewIDs = append(reviewIDs, id)
					} else if resp != nil {
						resp.Body.Close()
					}
				}
			}

		default: // Удалить отзыв
			if len(reviewIDs) > 0 {
				idx := rng.Intn(len(reviewIDs))
				id := reviewIDs[idx]
				resp, _ := post(client, reviewURL+"/reviews/delete?id="+strconv.Itoa(id))
				if resp != nil && resp.StatusCode == http.StatusNoContent {
					resp.Body.Close()
					reviewIDs[idx] = reviewIDs[len(reviewIDs)-1]
					reviewIDs = reviewIDs[:len(reviewIDs)-1]
				} else if resp != nil {
					resp.Body.Close()
				}
			}
		}

		time.Sleep(time.Duration(10+rng.Intn(60)) * time.Millisecond)
	}
}
