package whois

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type Info struct {
	Ip      string  `json:"ip"`
	Lat     float32 `json:"latitude"`
	Lon     float32 `json:"longitude"`
	City    string  `json:"city"`
	Country string  `json:"country"`
	/*
		   "ip": "142.250.68.14",
		   "success": true,
		   "type": "IPv4",
		   "continent": "North America",
		   "continent_code": "NA",
		   "country": "United States",
		   "country_code": "US",
		   "region": "California",
		   "region_code": "CA",
		   "city": "Los Angeles",
		   "is_eu": false,
		   "postal": "90012",
		   "calling_code": "1",
		   "capital": "Washington D.C.",
		   "borders": "CA,MX",

		"flag": {
			"img": "https://cdn.ipwhois.io/flags/us.svg",
			"emoji": "🇺🇸",
			"emoji_unicode": "U+1F1FA U+1F1F8"
		},

		"connection": {
			"asn": 15169,
			"org": "Google LLC",
			"isp": "Google LLC",
			"domain": "google.com"
		},

		"timezone": {
			"id": "America/Los_Angeles",
			"abbr": "PST",
			"is_dst": false,
			"offset": -28800,
			"utc": "-08:00",
			"current_time": "2025-02-21T17:33:00-08:00"
		}
	*/
}

func (info *Info) Longitude() float64 {
	return float64(info.Lon)
}

func (info *Info) Latitude() float64 {
	return float64(info.Lat)
}

func (info *Info) Address() string {
	return fmt.Sprintf("%s, %s", info.City, info.Country)
}

func Get() (info Info, err error) {
	var res *http.Response
	res, err = http.Get("https://ipwho.is/")
	if err != nil {
		return
	}
	defer res.Body.Close()

	dec := json.NewDecoder(res.Body)
	err = dec.Decode(&info)
	if err != nil {
		return
	}

	return
}
