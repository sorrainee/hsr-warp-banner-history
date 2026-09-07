package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"maps"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
)

const INDEXFILE = "index.json"
const INDEXMINFILE = "index.min.json"
const BANNERURL = "https://starrailstation.com/api/v1/warp_config"
const LCURL = "https://raw.githubusercontent.com/Mar-7th/StarRailRes/refs/heads/master/index_new/en/light_cones.json"
const CHARURL = "https://raw.githubusercontent.com/Mar-7th/StarRailRes/refs/heads/master/index_new/en/characters.json"

const CHARACTERGACHA = 11
const LCGACHA = 12
const COLLABCHARACTERGACHA = 21
const COLLABLCGACHA = 22
const STELLARGACHA = 1
const DEPARTUREGACHA = 2

const STELLARBOUNDARY = 2000
const CHARACTERBOUNDARY = 3000
const LCBOUNDARY = 4000
const DEPARTUREBOUNDARY = 5000
const COLLABCHARBOUNDARY = 6000

const VERSION = 4.4

type Response struct {
	Config struct {
		Banners map[int]struct {
			RateUp          int    `json:"rateup"`
			Sort            int    `json:"sort"`
			Rerun           int    `json:"rerun"`
			Accent          string `json:"accent"`
			Border          string `json:"border"`
			RateUp4         []int  `json:"rateup4"`
			IsExclusive     bool   `json:"is_exclusive"`
			StartTime       int    `json:"start_time"`
			EndTime         int    `json:"end_time"`
			CompanionBanner int    `json:"companion_banner"`
		}
		Cost           int
		Features       []string
		IsMaintenance  bool   `json:"is_maintenance"`
		MaintenanceMsg string `json:"maintenance_msg"`
		Sort           []int
		Types          map[any]any `json:"-"`
		WebCacheVer    string      `json:"webcache_ver"`
	}
	Time      int
	CompatVer int `json:"compat_ver"`
}

type BannerHistory struct {
	Banners []Banner `json:"banners"`
}

type Banner struct {
	Desc            string `json:"desc"`
	GachaType       string `json:"gacha_type"`
	Id              string `json:"id"`
	Version         string `json:"version"`
	RateUp4         []int  `json:"rateup4"`
	CompanionBanner int    `json:"companion_banner"`
	EndTime         int    `json:"end_time"`
	RateUp          int    `json:"rateup"`
	Rerun           int    `json:"rerun"`
	StartTime       int    `json:"start_time"`
	IsExclusive     bool   `json:"is_exclusive"`
}

type LightCone struct {
	Id       string
	Name     string
	Rarity   int
	Others map[string]any `json:"-"`
}

type Character struct {
	Id       string
	Name     string
	Rarity   int
	Others map[string]any `json:"-"`
}

func main() {
	monitor()
}

// Scheduled task to run twice a day every day to monitor starrailstation API and StarRailRes updates.
// Performs necessary updates to the index when updates are found.
func monitor() {
	index, err := readIndex()

	if err != nil {
		log.Fatalln("Unable to read " + INDEXFILE + ". ERROR: " + err.Error())
		return
	}
	
	resp, err := http.Get(BANNERURL)

	if err != nil {
		log.Fatalln("Unable to get a response from starrailstation API. ERROR: " + err.Error())
		return
	}

	defer resp.Body.Close()
		
	lcResp, err := http.Get(LCURL)

	if err != nil {
		log.Fatalln("Unable to request light_cones.json from StarRailRes repository")
		return
	}

	defer lcResp.Body.Close()

	charResp, err := http.Get(CHARURL)

	if err != nil {
		log.Fatalln("Unable to request characters.json from StarRailRes repository")
		return
	}

	defer charResp.Body.Close()
	
	var response *Response
	var lcResponse map[string]LightCone
	var charResponse map[string]Character
	
	decoder := json.NewDecoder(resp.Body)
	decoder.Decode(&response)

	b, err := io.ReadAll(lcResp.Body)

	if err != nil {
		log.Fatalln("Unable to read StarRailRes light_cones.json content")
		return
	}

	json.Unmarshal(b, &lcResponse)

	b, err = io.ReadAll(charResp.Body)

	if err != nil {
		log.Fatalln("Unable to read StarRailRes characters.json content")
		return
	}

	json.Unmarshal(b, &charResponse)
	
	charIndex, _ := strconv.Atoi(getLatestBanner(index, CHARACTERGACHA).Id)
	lcIndex, _ := strconv.Atoi(getLatestBanner(index, LCGACHA).Id)
	collabCharIndex, _ := strconv.Atoi(getLatestBanner(index, COLLABCHARACTERGACHA).Id)
	collabLcIndex, _ := strconv.Atoi(getLatestBanner(index, COLLABLCGACHA).Id)

	indices := []int{charIndex, lcIndex, collabCharIndex, collabLcIndex}

	changed := false

	for _, idx := range indices {
		count := 1
		gtype := COLLABLCGACHA

		if idx > STELLARBOUNDARY && idx < CHARACTERBOUNDARY {
			gtype = CHARACTERGACHA
		} else if idx < LCBOUNDARY {
			gtype = LCGACHA
		} else if idx < DEPARTUREBOUNDARY {
			gtype = DEPARTUREGACHA
		} else if idx < COLLABCHARBOUNDARY {
			gtype = COLLABCHARACTERGACHA
		}

		gachaType := strconv.Itoa(gtype)
		
		in:
		for {
			latest, exists := response.Config.Banners[idx + count]

			if !exists {
				break in
			}

			banner := &Banner{
				Id : strconv.Itoa(idx + count),
				RateUp4: latest.RateUp4,
				RateUp: latest.RateUp,
				Rerun: latest.Rerun,
				GachaType: gachaType,
				CompanionBanner: latest.CompanionBanner,
				StartTime: latest.StartTime,
				EndTime: latest.EndTime,
				IsExclusive: true,
			}

			err = addBanner(index, lcResponse, charResponse, banner)

			if err == nil {
				changed = true
			}
			
			count++
		}
	}

	if changed {
		content, err := json.MarshalIndent(index, "", "    ")
		miniContent, err := json.Marshal(index)
	
		if err != nil {
			log.Fatalln("Unable to marshal content")
			return
		}
	
		err = writeToIndex(&content, &miniContent)
	
		if err != nil {
			log.Fatalln("Unable to write to " + INDEXFILE)
			return
		}

		log.Println("Successfully updated " + INDEXFILE)
	}
}

// Writes content to index.json and index.min.json
func writeToIndex(content *[]byte, miniContent *[]byte) error {
	cwd, err := os.Getwd()

	if err != nil {
		return err
	}

	file, err := os.OpenFile(cwd+"/"+INDEXFILE, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0666)
	miniFile, err := os.OpenFile(cwd+"/"+INDEXMINFILE, os.O_RDWR|os.O_TRUNC|os.O_CREATE, 0666)

	if err != nil {
		return err
	}

	defer file.Close()

	_, err = file.Write(*content)
	_, err = miniFile.Write(*miniContent)

	if err != nil {
		return err
	}

	return nil
}

// Returns the structured json content from index.json
func readIndex() (*BannerHistory, error) {
	var index BannerHistory

	cwd, err := os.Getwd()

	if err != nil {
		return &index, err
	}

	content, err := os.ReadFile(cwd + "/" + INDEXFILE)

	if err != nil {
		return &index, err
	}

	err = json.Unmarshal(content, &index)

	if err != nil {
		return &index, err
	}

	return &index, nil
}

// Gets the latest banner for each GachaType.
// Returns a placeholder banner holding a dummy ID if no banner was found for a GachaType
func getLatestBanner(index *BannerHistory, gachaType int) *Banner {
	bannerIndex := slices.Clone(index.Banners)

	bannerIndex = slices.DeleteFunc(bannerIndex, func(b Banner) bool {
		id, _ := strconv.Atoi(b.Id)

		cond := false
		switch gachaType {
		case CHARACTERGACHA:
			cond = id > CHARACTERBOUNDARY-1
		case LCGACHA:
			cond = id < CHARACTERBOUNDARY || id > LCBOUNDARY-1
		case COLLABCHARACTERGACHA:
			cond = id < DEPARTUREBOUNDARY || id > COLLABCHARBOUNDARY-1
		case COLLABLCGACHA:
			cond = id < COLLABCHARBOUNDARY
		}

		return cond
	})

	bannerLength := len(bannerIndex)

	if bannerLength > 0 {
		return &bannerIndex[bannerLength-1]
	} else {
		id := 0
		
		switch gachaType {
		case CHARACTERGACHA:
			id = CHARACTERBOUNDARY-998
		case LCGACHA:
			id = LCBOUNDARY-998
		case COLLABCHARACTERGACHA:
			id = COLLABCHARBOUNDARY-1000
		case COLLABLCGACHA:
			id = COLLABCHARBOUNDARY
		}
		return &Banner{
			Id: strconv.Itoa(id),
		}
	}
}

// Gets the banner by Id
func getBannerById(index *BannerHistory, id string) *Banner {
	idx := slices.IndexFunc(index.Banners, func(b Banner) bool {
		return b.Id == id
	})

	return &index.Banners[idx]
}

// Gets the banner by RateUp
func getBannerByRateUp(index *BannerHistory, rateUp int) *Banner {
	idx := slices.IndexFunc(index.Banners, func(b Banner) bool {
		return b.RateUp == rateUp
	})

	return &index.Banners[idx]
}

// Adds banner as the latest banner of the banner GachaType
// Should be used when new banners come out and after index.json is initialized with setup()
func addBanner(index *BannerHistory, lcResponse map[string]LightCone, charResponse map[string]Character,  banner *Banner) error {
	gachaType, _ := strconv.Atoi(banner.GachaType)
	id, _ := strconv.Atoi(banner.Id)

	latestBanner := getLatestBanner(index, gachaType)
	latestId, _ := strconv.Atoi(latestBanner.Id)

	if id > 0 && latestId >= id {
		return errors.New("Latest Banner ID is equal to or more recent than the input banner ID")
	}

	latestIndex := 0

	if latestId % 1000 == 0 || ((latestId - 2) % 1000 == 0 && (gachaType == CHARACTERGACHA || gachaType == LCGACHA)) {
		id := 0
		lastId, _ := strconv.Atoi(index.Banners[len(index.Banners)-1].Id)

		if lastId <= id {
			latestIndex = len(index.Banners)-1
		}
		
		switch gachaType {
		case CHARACTERGACHA:
			id = CHARACTERBOUNDARY-1000
			latestIndex = slices.IndexFunc(index.Banners, func(b Banner) bool {
				gt, _ := strconv.Atoi(b.GachaType)
				return gt == DEPARTUREGACHA
			})
		case LCGACHA:
			id = LCBOUNDARY-1000
			latestIndex = slices.IndexFunc(index.Banners, func(b Banner) bool {
				return b.Id == getLatestBanner(index, CHARACTERGACHA).Id
			})
		case COLLABCHARACTERGACHA:
			id = COLLABCHARBOUNDARY-1000
		case COLLABLCGACHA:
			id = COLLABCHARBOUNDARY
		}
	} else {
		latestIndex = slices.IndexFunc(index.Banners, func(b Banner) bool {
			return b.Id == latestBanner.Id
		})
	}

	switch gachaType {
		case CHARACTERGACHA:
			if banner.Rerun > 0 {
				banner.Desc = strings.Join([]string{"Indelible Coterie", charResponse[strconv.Itoa(banner.RateUp)].Name}, ": ")
			}
		case LCGACHA, COLLABLCGACHA:
			banner.Desc = lcResponse[strconv.Itoa(banner.RateUp)].Name

			if banner.Rerun > 0 {
				banner.Desc = strings.Join([]string{"Coalesced Truths", banner.Desc}, ": ")
			}
	}
	
	banner.Version = strconv.FormatFloat(VERSION, 'f', 1,32)
	
	index.Banners = slices.Insert(index.Banners, latestIndex + 1, *banner)
	
	return nil
}


// -- Not needed other than initialization -- //

// One-time function for generating the index straight from the sources
func _setup() {
	var wg sync.WaitGroup

	index := &BannerHistory{Banners: make([]Banner, 0)}

	index, err := readIndex()

	if err != nil {
		log.Fatalln("Unable to read index.json. ERROR: " + err.Error())
		return
	}

	wg.Go(func() {
		var response Response

		resp, err := http.Get(BANNERURL)

		if err != nil {
			log.Fatalln("Unable to get a response from starrailstation API. ERROR: " + err.Error())
			return
		}

		defer resp.Body.Close()

		decoder := json.NewDecoder(resp.Body)

		decoder.Decode(&response)

		for k, v := range response.Config.Banners {
			gtype := COLLABLCGACHA

			if k < STELLARBOUNDARY {
				gtype = STELLARGACHA
			} else if k < CHARACTERBOUNDARY {
				gtype = CHARACTERGACHA
			} else if k < LCBOUNDARY {
				gtype = LCGACHA
			} else if k < DEPARTUREBOUNDARY {
				gtype = DEPARTUREGACHA
			} else if k < COLLABCHARBOUNDARY {
				gtype = COLLABCHARACTERGACHA
			}

			banner := &Banner{
				GachaType:       strconv.Itoa(gtype),
				Id:              strconv.Itoa(k),
				RateUp4:         v.RateUp4,
				RateUp:          v.RateUp,
				Rerun: v.Rerun,
				EndTime:         v.EndTime,
				StartTime:       v.StartTime,
				CompanionBanner: v.CompanionBanner,
				IsExclusive:     v.IsExclusive,
			}


			bannerIdx := slices.IndexFunc(index.Banners, func(b Banner) bool {
				return b.Id == banner.Id
			})

			if bannerIdx > -1 {
				banner.Desc = index.Banners[bannerIdx].Desc
				banner.Version = index.Banners[bannerIdx].Version
			} else {
				index.Banners = append(index.Banners, *banner)
			}
		}

		// Sort ascending order by id first
		slices.SortStableFunc(index.Banners, func(i, j Banner) int {
			ival, _ := strconv.Atoi(i.Id)
			jval, _ := strconv.Atoi(j.Id)

			return ival - jval
		})

		// and then by gacha type so it becomes stellar > departure > event char > event lc > collab char > collab lc
		slices.SortStableFunc(index.Banners, func(i, j Banner) int {
			ival, _ := strconv.Atoi(i.GachaType)
			jval, _ := strconv.Atoi(j.GachaType)

			return ival - jval
		})
	})

	wg.Go(func() {
		lcResp, err := http.Get(LCURL)

		if err != nil {
			log.Fatalln("Unable to request light_cones.json from StarRailRes repository")
			return
		}

		defer lcResp.Body.Close()

		var lcResponse map[string]LightCone

		content, err := io.ReadAll(lcResp.Body)

		if err != nil {
			log.Fatalln("Unable to read light_cones.json content")
			return
		}

		json.Unmarshal(content, &lcResponse)

		lcIds := make(map[int]int, 0)

		for _, banner := range index.Banners {
			id, _ := strconv.Atoi(banner.Id)

			if id > 3000 && id < 4000 {
				bannerIdx := slices.IndexFunc(index.Banners, func(cmpBanner Banner) bool {
					return cmpBanner.Id == banner.Id
				})

				append := ""

				if !slices.Contains(slices.Collect(maps.Keys(lcIds)), banner.RateUp) {
					lcIds[banner.RateUp] = 0
				} else {
					lcIds[banner.RateUp] = lcIds[banner.RateUp] + 1
					banner.Rerun = lcIds[banner.RateUp]
				}

				if lcIds[banner.RateUp] > 0 {
					append = "Coalesced Truths: "
				}

				lcId := strconv.Itoa(banner.RateUp)

				banner.Desc = append + lcResponse[lcId].Name

				index.Banners[bannerIdx] = banner
			}
		}
	})

	wg.Wait()

	content, err := json.MarshalIndent(index, "", "    ")
	miniContent, err := json.Marshal(index)

	if err != nil {
		log.Fatalln("Unable to marshal content")
		return
	}

	err = writeToIndex(&content, &miniContent)

	if err != nil {
		log.Fatalln("Unable to write to index.json")
		return
	}
}

// Sets the version of the light cones appropriately with their respective character banners.
// For index generation purposes only.
func _applyVersionToLc(index *BannerHistory) {
	charIndex := slices.Clone(index.Banners)
	lcIndex := slices.Clone(index.Banners)

	charIndex = slices.DeleteFunc(charIndex, func(b Banner) bool {
		id, _ := strconv.Atoi(b.Id)
		return id > CHARACTERBOUNDARY-1
	})

	lcIndex = slices.DeleteFunc(lcIndex, func(b Banner) bool {
		id, _ := strconv.Atoi(b.Id)
		return id < CHARACTERBOUNDARY || id > LCBOUNDARY-1
	})

	for i, j := range charIndex {
		index.Banners[i+len(charIndex)+1].Version = j.Version
	}
}
