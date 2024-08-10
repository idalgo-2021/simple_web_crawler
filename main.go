package main

import (
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/exp/rand"
)

func getRequest(targetUrl string) (*http.Response, error) {
	client := &http.Client{}
	req, _ := http.NewRequest("GET", targetUrl, nil)
	req.Header.Set("User-Agent", randomUserAgent())
	return client.Do(req)
}

func randomUserAgent() string {
	userAgents := []string{
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/61.0.3163.100 Safari/537.36",
		"Mozilla/5.0 (Windows NT 6.1; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/61.0.3163.100 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/61.0.3163.100 Safari/537.36",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_12_6) AppleWebKit/604.1.38 (KHTML, like Gecko) Version/11.0 Safari/604.1.38",
		"Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:56.0) Gecko/20100101 Firefox/56.0",
		"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_13) AppleWebKit/604.1.38 (KHTML, like Gecko) Version/11.0 Safari/604.1.38",
	}
	return userAgents[rand.Intn(len(userAgents))]
}

func discoverLinks(response *http.Response, baseURL string) []string {
	doc, _ := goquery.NewDocumentFromReader(response.Body)
	foundUrls := []string{}

	if doc != nil {
		doc.Find("a").Each(func(i int, s *goquery.Selection) {
			href, _ := s.Attr("href")
			if link, ok := resolveRelativeLinks(href, baseURL); ok {
				foundUrls = append(foundUrls, link)
			}
		})
	}
	return foundUrls
}

func resolveRelativeLinks(href string, baseURL string) (string, bool) {
	parsedBase, _ := url.Parse(baseURL)
	parsedHref, _ := url.Parse(href)

	if !parsedHref.IsAbs() {
		parsedHref = parsedBase.ResolveReference(parsedHref)
	}

	if parsedBase.Host == parsedHref.Host {
		return parsedHref.String(), true
	}
	return "", false
}

func crawler(address string, linksMap *sync.Map, wg *sync.WaitGroup, semaphore chan struct{}) {

	defer wg.Done()

	semaphore <- struct{}{} // Занимаем слот

	resp, err := getRequest(address)
	if err != nil {
		fmt.Printf("Failed to fetch %s: %v\n", address, err)
		<-semaphore // Освобождаем слот
		return
	}
	defer resp.Body.Close()

	links := discoverLinks(resp, address)
	for _, link := range links {
		_, loaded := linksMap.LoadOrStore(link, true)
		if !loaded {
			wg.Add(1)
			go crawler(link, linksMap, wg, semaphore)
		}
	}
	<-semaphore // Освобождаем слот

}

func savingLinksToFile(address string, linksMap *sync.Map) error {
	parsedURL, err := url.Parse(address)
	if err != nil {
		return err
	}

	filename := parsedURL.Hostname() + ".csv"
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	linksMap.Range(func(key, value interface{}) bool {
		link := key.(string)
		err := writer.Write([]string{link})
		return err == nil
	})

	return nil
}

func domainTraversal(address string) error {
	trimmedAddress := strings.TrimSpace(address)
	if trimmedAddress == "" {
		return errors.New("domain name cannot be empty")
	}

	linksMap := &sync.Map{}
	semaphore := make(chan struct{}, 10) // Число слотов для горутин, одновременно обрабатывающих найденные ссылки

	var wg sync.WaitGroup
	wg.Add(1)
	go crawler(trimmedAddress, linksMap, &wg, semaphore)
	wg.Wait()

	linksCount := 0
	linksMap.Range(func(key, value interface{}) bool {
		linksCount++
		return true
	})
	if linksCount == 0 {
		return errors.New("no links found")
	}

	if err := savingLinksToFile(trimmedAddress, linksMap); err != nil {
		return err
	}

	return nil
}

func main() {
	goalAddress := "https://example.com"

	err := domainTraversal(goalAddress)
	if err != nil {
		fmt.Printf("error: %v\n", err)

	} else {
		fmt.Println("Process successed!")
	}
}
