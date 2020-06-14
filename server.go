package main

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/iterator"
)

func main() {
	mux := http.NewServeMux()
	files := http.FileServer(http.Dir("/public"))
	mux.Handle("/static/", http.StripPrefix("/static/", files))
	mux.HandleFunc("/", index)
	server := &http.Server{
		Addr:    "0.0.0.0:8080",
		Handler: mux,
	}
	server.ListenAndServe()
}

func index(writer http.ResponseWriter, request *http.Request) {
	template, _ := template.ParseFiles("templates/index.html")

	ctx := context.Background()

	client, err := bigquery.NewClient(ctx, "chunlin-lunar")
	if err != nil {
		log.Fatalf("bigquery.NewClient: %v", err)
	}
	defer client.Close()

	rows, err := queryGlobalData(ctx, client)

	if err != nil {
		log.Fatal(err)
	}

	globalQueryResult := processGlobalQueryResult(rows)

	result := DisplayResult{
		TopCountries: globalQueryResult[:10],
		AllCountries: globalQueryResult,
	}

	for i := 0; i < len(globalQueryResult); i++ {
		if globalQueryResult[i].CountryRegion == "Singapore" {
			result.SingaporeConfirmedCases = globalQueryResult[i].ConfirmedCases
			result.SingaporeDeaths = globalQueryResult[i].Deaths
		}

		if globalQueryResult[i].CountryRegion == "Malaysia" {
			result.MalaysiaConfirmedCases = globalQueryResult[i].ConfirmedCases
			result.MalaysiaDeaths = globalQueryResult[i].Deaths
		}
	}

	rows, err = queryDailyCaseData(ctx, client, "Singapore")

	if err != nil {
		log.Fatal(err)
	}

	dailyCaseQueryResult := processDailyCaseQueryResult(rows)

	result.SingaporeCases = dailyCaseQueryResult

	rows, err = queryDailyCaseData(ctx, client, "Malaysia")

	if err != nil {
		log.Fatal(err)
	}

	dailyCaseQueryResult = processDailyCaseQueryResult(rows)

	result.MalaysiaCases = dailyCaseQueryResult

	template.Execute(writer, result)
}

func queryGlobalData(ctx context.Context, client *bigquery.Client) (*bigquery.RowIterator, error) {
	dateRange := ""

	dataStartDate := time.Date(2020, 1, 22, 0, 0, 0, 0, time.UTC)
	dataEndDate := time.Now().AddDate(0, 0, -1)
	for d := dataStartDate; d.After(dataEndDate) == false; d = d.AddDate(0, 0, 1) {
		dateRange += fmt.Sprintf("_%v_%v_%v", int(d.Month()), d.Day(), d.Year()-2000)

		if !d.AddDate(0, 0, 1).After(dataEndDate) {
			dateRange += " + "
		}
	}

	latestDate := time.Now().AddDate(0, 0, -2)
	latestDateInQuery := fmt.Sprintf("_%v_%v_%v", int(latestDate.Month()), latestDate.Day(), latestDate.Year()-2000)

	queryForConfirmedCases := `(SELECT
		IF(province_state IS NULL, "", CONCAT(province_state, ", ", country_region)) AS place,
		country_region,
		latitude,
		longitude,
		(` + latestDateInQuery + `) AS count
	  FROM ` + "`bigquery-public-data.covid19_jhu_csse.confirmed_cases`) AS ConfirmedCases"

	queryForDeaths := `(SELECT
		IF(province_state IS NULL, "", CONCAT(province_state, ", ", country_region)) AS place,
		country_region,
		latitude,
		longitude,
		(` + latestDateInQuery + `) AS count
	  FROM ` + "`bigquery-public-data.covid19_jhu_csse.deaths`) AS Deaths"

	query := client.Query(
		`SELECT 
				ConfirmedCases.place, 
				ConfirmedCases.country_region, 
				ConfirmedCases.latitude, 
				ConfirmedCases.longitude, 
				ConfirmedCases.count as confirmed_cases, 
				Deaths.count As deaths
			FROM ` + queryForConfirmedCases + `
			LEFT JOIN ` + queryForDeaths + ` 
			ON ConfirmedCases.place = Deaths.place AND ConfirmedCases.country_region = Deaths.country_region
			ORDER BY ConfirmedCases.count DESC;`)
	return query.Read(ctx)
}

func queryDailyCaseData(ctx context.Context, client *bigquery.Client, countryRegion string) (*bigquery.RowIterator, error) {
	query := client.Query(
		`SELECT 
			CAST(date as STRING) as date, 
			IFNULL(confirmed,0) as confirmed_cases, 
			IFNULL(deaths, 0) as deaths
		FROM ` + "`bigquery-public-data.covid19_jhu_csse.summary`" + `
		WHERE country_region = '` + countryRegion + `'
		ORDER BY date;`)

	return query.Read(ctx)
}

type DisplayResult struct {
	SingaporeConfirmedCases int64
	SingaporeDeaths         int64
	SingaporeCases          []DailyCaseQueryResultDataRow
	MalaysiaConfirmedCases  int64
	MalaysiaDeaths          int64
	MalaysiaCases           []DailyCaseQueryResultDataRow
	TopCountries            []GlobalQueryResultDataRow
	AllCountries            []GlobalQueryResultDataRow
}

type GlobalQueryResultDataRow struct {
	Place          string  `bigquery:"place"`
	CountryRegion  string  `bigquery:"country_region"`
	Latitude       float64 `bigquery:"latitude"`
	Longitude      float64 `bigquery:"longitude"`
	ConfirmedCases int64   `bigquery:"confirmed_cases"`
	Deaths         int64   `bigquery:"deaths"`
}

type DailyCaseQueryResultDataRow struct {
	Date           string `bigquery:"date"`
	ConfirmedCases int64  `bigquery:"confirmed_cases"`
	Deaths         int64  `bigquery:"deaths"`
}

func processGlobalQueryResult(iter *bigquery.RowIterator) []GlobalQueryResultDataRow {
	var result []GlobalQueryResultDataRow

	for {
		var row GlobalQueryResultDataRow

		err := iter.Next(&row)

		if err == iterator.Done {
			break
		}

		if err != nil {
			log.Print(err)
			continue
		}

		result = append(result, row)
	}

	return result
}

func processDailyCaseQueryResult(iter *bigquery.RowIterator) []DailyCaseQueryResultDataRow {
	var result []DailyCaseQueryResultDataRow

	for {
		var row DailyCaseQueryResultDataRow

		err := iter.Next(&row)

		if err == iterator.Done {
			break
		}

		if err != nil {
			log.Print(err)
			continue
		}

		result = append(result, row)
	}

	return result
}
