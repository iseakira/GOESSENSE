package main

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/encoding/japanese"
)

type Entry struct {
	AuthorID string
	Author   string
	TitleID  string
	Title    string
	InfoURL  string
	ZipURL   string
}


func findAuthorAndZIP(siteURL string)(string,string){
	log.Println("query",siteURL)
	doc,err := goquery.NewDocument(siteURL)
	if err != nil {
		return "",""
	}
	author := doc.Find("table[summary=作家データ]tr:nth-child(1)td:nth-child(2)").Text()
	zipURL := ""
	doc.Find("table.download a").Each(func(n int,elem *goquery.Selection){
		href := elem.AttrOr("href","")
		if strings.HasSuffix(href,"zip"){
			zipURL = href
		}
	})
	if zipURL == ""{
		return author,""
	}
	if strings.HasPrefix(zipURL,"http://")||strings.HasPrefix(zipURL,"https://"){
		return author,zipURL
	}
	u,err := url.Parse(siteURL)
	if err != nil {
		return author,""
	}
	u.Path=path.Join(path.Dir(u.Path),zipURL)
	return author,u.String()
}

func findEntries(siteURL string) ([]Entry, error) {
	doc,err := goquery.NewDocument(siteURL)
	if err != nil {
		return nil,err
	}
	//Eachでいっこずつfuncにわたす(nはインデックス、elemは内容)
	//patは正規表現で番号のみを()で抽出できるような前処理
	//FindStrinfSubmatchでhref以降のurlぬきとって全体一致と番号をそれぞれ抜いてる
	entries := []Entry{}
	pat := regexp.MustCompile(`.*/cards/([0-9]+)/card([0-9]+).html$`)
	doc.Find("ol li a").Each(func(n int,elem *goquery.Selection){
		token := pat.FindStringSubmatch(elem.AttrOr("href",""))
		if len(token) != 3{
			return
		}
		title := elem.Text()
		pageURL := fmt.Sprintf("https://www.aozora.gr.jp/cards/%s/card%s.html",token[1],token[2])
		author,zipURL := findAuthorAndZIP(pageURL)
		if zipURL != "" {
			entries = append(entries,Entry{
				AuthorID : token[1],
				Author: author,
				TitleID:token[2],
				Title:title,
				InfoURL:siteURL,
				ZipURL:zipURL,

			})
		}
		println(pageURL)
		println(author)
		println(zipURL)
	})
	return entries,nil
}


//resp *Response型→response.Body io.reader型→b []byte→r,ReaderAt型
func extractText(zipURL string)(string,error){
	//responseをhttp.Getで取得
	resp,err:= http.Get(zipURL)
	if err != nil {
		return "",err
	}
	defer resp.Body.Close()

  //responseのbodyをbに乗っける
	//このときresponse.Bodyはio.reader
	//ioutil.ReadALLでbyteにする
	b,err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "",err
	}

	//[]fileをrに返す
	//bytes.NewReader(b)によって[]byteからランダムアクセス可能なReaderAtを作る
	//int64は全体のサイズ(バイト数)
	r,err := zip.NewReader(bytes.NewReader(b),int64(len(b)))

	if err != nil {
		return "",err
	}

	for _, file := range r.File {
		if path.Ext(file.Name) == ".txt" {
			f,err := file.Open()
			if err != nil {
				return "",err
			}
			b,err := ioutil.ReadAll(f)
			f.Close()
			if err != nil {
				return "",err
			}
			b,err = japanese.ShiftJIS.NewDecorder().Bytes(b)
			if err != nil {
				return "",err
			}
			return string(b),nil
		}
	}
	return "",errors.New("contents not found")

}

func main() {
	listURL := "https://www.aozora.gr.jp/index_pages/person879.html"

	entries, err := findEntries(listURL)
	if err != nil {
		log.Fatal(err)
	}
	for _, entry := range entries {
		content, err := extractText(entry.ZipURL)
		if err != nil {
			log.Println(err)
			continue
		}
		fmt.Println(entry.InfoURL)
		fmt.Println(content)
	}
}