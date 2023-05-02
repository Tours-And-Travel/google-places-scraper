package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

type Place struct {
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Address     string   `json:"address"`
	Website     string   `json:"website"`
	Phone       string   `json:"phone"`
	ReviewCount int      `json:"review_count"`
	Stars       float64  `json:"stars"`
	FiveStars   int      `json:"5_stars"`
	FourStars   int      `json:"4_stars"`
	ThreeStars  int      `json:"3_stars"`
	TwoStars    int      `json:"2_stars"`
	OneStar     int      `json:"1_star"`
	Reviews     []string `json:"reviews"`
	LatLon      string   `json:"latlon"`
}

func getStarsValue(str string) int {
	splitString := strings.Split(str, " ")
	num := toInt(splitString[2])
	return num
}

func toFloat(str string) float64 {
	value, err := strconv.ParseFloat(str, 64)

	if err != nil {
		return 0
	}

	return value
}

func toInt(str string) int {
	// Remove the comma from the input string
	cleanedInput := strings.ReplaceAll(str, ",", "")

	// Convert the cleaned string to an integer
	value, err := strconv.Atoi(cleanedInput)

	if err != nil {
		return 0
	}

	return value
}

func getLatLon(urlString string) string {
	atIndex := strings.Index(urlString, "@")
	slashIndex := strings.LastIndex(urlString, "/")

	if atIndex != -1 && slashIndex != -1 && slashIndex > atIndex {
		str := urlString[atIndex+1 : slashIndex]
		parts := strings.Split(str, ",")
		lat := parts[0]
		lon := parts[1]
		return fmt.Sprintf("%s,%s", lat, lon)
	}

	return ""
}

func formatURL(name string) string {
	return fmt.Sprintf("https://www.google.com/maps/search/%v/?hl=en", strings.ReplaceAll(name, " ", "+"))
}

func doFirstTwoWordsMatch(sentence1, sentence2 string) bool {
	// Split the sentences into words
	words1 := strings.Fields(sentence1)
	words2 := strings.Fields(sentence2)

	// Check if the first two words match
	if len(words1) >= 2 && len(words2) >= 2 {
		return strings.EqualFold(words1[0], words2[0]) && strings.EqualFold(words1[1], words2[1])
	}

	return false
}

func crawlPlaces(browser *rod.Browser, input string, ch chan *Place) {
	page := browser.MustPage()
	url := formatURL(input)
	fmt.Print("Visiting: ", url, "\n")
	page.MustNavigate(url)
	page.MustWaitLoad()
	currentURL := page.MustInfo().URL
	count := 0

	for strings.Contains(currentURL, "maps/search") {
		count += 1
		currentURL = page.MustInfo().URL

		if currentURL != url && strings.Contains(currentURL, "maps/search") {
			links := page.MustElementsX(fmt.Sprintf("//*[contains(@href,'%s')]", "maps/place"))

			for _, hyper := range links {
				label := *hyper.MustAttribute("aria-label")

				if doFirstTwoWordsMatch(input, label) {
					page.MustNavigate(*hyper.MustAttribute("href"))
					page.MustWaitLoad()
					break
				}
			}
		}

		if strings.Contains(currentURL, "maps/place") {
			break
		}

		if count > 30 {
			break
		}

		time.Sleep(1 * time.Second)
	}

	// Skip if place not found
	if strings.Contains(currentURL, "maps/search") {
		fmt.Println("Skipping: ", input, ", place not found.")
		ch <- nil
	} else {
		fmt.Println("CurrentURL: ", currentURL)

		place := getPlaceDetails(page, currentURL)
		ch <- &place
	}
}

func main() {
	inputs := []string{
		// place names here
		"Laba africa expeditions",
	}

	places := make(chan *Place, len(inputs))
	results := make([]Place, 0)

	browser := rod.New().MustConnect()
	defer browser.MustClose()

	for _, input := range inputs {
		go crawlPlaces(browser, input, places)
	}

	for i := 0; i < len(inputs); i++ {
		place := <-places

		if place != nil {
			results = append(results, *place)
		}
	}

	if jsonData, err := json.Marshal(results); err == nil {
		jsonFile, err := os.Create("places.json")

		if err != nil {
			panic(err)
		}

		defer jsonFile.Close()

		if _, err = jsonFile.WriteString(string(jsonData)); err != nil {
			panic(err)
		}
	}

	fmt.Println("Done!")
}

func getPlaceDetails(page *rod.Page, currentURL string) Place {
	name := strings.TrimSpace(page.MustElement("h1").MustText())
	category := strings.TrimSpace(page.MustElement("button[jsaction='pane.rating.category']").MustText())

	address := ariaNoLabel(page, "Address: ")
	website := ariaNoLabel(page, "Website: ")
	phone := ariaNoLabel(page, "Phone: ")
	reviewCount := ariaNoLabel(page, " reviews")

	stars := ariaWithLabel(page, " stars")

	starsValue := regexp.MustCompile(`\d+.*\d+`).FindString(stars)

	fiveStars := ariaWithLabel(page, "5 stars")
	fiveStarsValue := getStarsValue(fiveStars)

	fourStars := ariaWithLabel(page, "4 stars")
	fourStarsValue := getStarsValue(fourStars)

	threeStars := ariaWithLabel(page, "3 stars")
	threeStarsValue := getStarsValue(threeStars)

	twoStars := ariaWithLabel(page, "2 stars")
	twoStarsValue := getStarsValue(twoStars)

	oneStar := ariaWithLabel(page, "1 star")
	oneStarValue := getStarsValue(oneStar)

	place := Place{
		Name:        name,
		Category:    category,
		Address:     address,
		Website:     website,
		Phone:       phone,
		ReviewCount: toInt(reviewCount),
		Stars:       toFloat(starsValue),
		FiveStars:   fiveStarsValue,
		FourStars:   fourStarsValue,
		ThreeStars:  threeStarsValue,
		TwoStars:    twoStarsValue,
		OneStar:     oneStarValue,
		LatLon:      getLatLon(currentURL),
		Reviews:     []string{},
	}

	// goroutine here
	moreReviewsQuery := fmt.Sprintf("//*[contains(@aria-label,'%s')]", "More reviews")

	if elementIsAvailable(page, moreReviewsQuery) {
		viewMoreReviews := page.MustElementX(moreReviewsQuery)
		reviewsElStr := "[jsaction='mouseover:pane.review.in; mouseout:pane.review.out']"

		viewMoreReviews.MustClick()
		// Wait for the page to react to the click event
		var count int = len(page.MustElements(reviewsElStr))

		fmt.Println("Scroling review pages...")

		// Scroll through all reviews
		for count < place.ReviewCount {
			count = len(page.MustElements(reviewsElStr))
			fmt.Printf("Scrolling: %v/%v\n", count, place.ReviewCount)
			page.Mouse.MustScroll(0, 10000)
			time.Sleep(1 * time.Second)
		}
	}

	if reviewDivs, _ := page.Elements("div.MyEned"); len(reviewDivs) > 0 {
		fmt.Println("Parsing reviews...")

		for _, div := range reviewDivs {
			if expandReview, err := div.Element("[jsaction='pane.review.expandReview']"); err == nil {
				expandReview.MustClick()
				// Wait for the page to react to the click event
				time.Sleep(2 * time.Second)
			}

			if span, err := div.Element("span"); err == nil {
				comment := span.MustText()
				place.Reviews = append(place.Reviews, strings.ReplaceAll(strings.TrimSpace(comment), "\u0026", ""))
			}
		}
	}

	// end here

	return place
}

func elementIsAvailable(page *rod.Page, q string) bool {
	err := rod.Try(func() {
		page.Timeout(1 * time.Second).MustElementX(q)
	})

	return !errors.Is(err, context.DeadlineExceeded)
}

func ariaWithLabel(page *rod.Page, label string) string {
	q := fmt.Sprintf("//*[contains(@aria-label,'%s')]", label)

	if elementIsAvailable(page, q) {
		elem := page.MustElementX(q)
		str, _ := elem.Attribute("aria-label")
		return *str
	}

	return ""
}

func ariaNoLabel(page *rod.Page, label string) string {
	text := ariaWithLabel(page, label)
	return strings.TrimSpace(strings.ReplaceAll(text, label, ""))
}
