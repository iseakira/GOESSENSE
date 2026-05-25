package main

import (
	"archive/zip"
	"bytes"
	"database/sql"
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
	"github.com/ikawaha/kagome-dict/ipa"
	"github.com/ikawaha/kagome/v2/tokenizer"
	"golang.org/x/text/encoding/japanese"
	_ "modernc.org/sqlite"
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
			b,err = japanese.ShiftJIS.NewDecoder().Bytes(b)
			if err != nil {
				return "",err
			}
			return string(b),nil
		}
	}
	return "",errors.New("contents not found")

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

//ファイルパスをぶち込むとDBハンドルを返す
func setupDB(dsn string)(*sql.DB,error){
	db,err := sql.Open("sqlite",dsn)
	if err != nil {
		return nil,err
	}

	//contents_ftsという全文検索用の仮想テーブルの作成
	queries := []string {
		`CREATE TABLE IF NOT EXISTS authors(
		author_id TEXT,
		 author TEXT,
		 PRIMARY KEY(author_id))`,
	  `CREATE TABLE IF NOT EXISTS contents(
		author_id TEXT,
		title_id TEXT,
		title TEXT,
		content TEXT,
		PRIMARY KEY (author_id, title_id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS contents_fts USING fts5(words)`,
	}
	for _,query := range queries {
		_,err = db.Exec(query)
		if err != nil {
			return nil,err
		}
	}
	return db,nil

}

func addEntry(db *sql.DB, entry *Entry, content string)error{
	_,err := db.Exec(`
         REPLACE INTO authors(author_id, author) values(?, ?) `,
         entry.AuthorID,
         entry.Author,)
	if err != nil {
		return err
	}
	res,err := db.Exec(
		`REPLACE INTO contents(author_id, title_id, title, content) values(?, ?, ?, ?)`,
         entry.AuthorID,
         entry.TitleID,
         entry.Title,
         content,
	)
	if err != nil {
		return err
	}
	//IDを取得
	docID,err := res.LastInsertId()
	if err != nil {
		return err
	}
	//tはけいたいそかいせき本体をつくる
	t,err := tokenizer.New(ipa.Dict(),tokenizer.OmitBosEos())
	if err != nil {
		return err
	}
	//わかちがきをsegに保存
	seg := t.Wakati(content)
	_,err = db.Exec(
		` REPLACE INTO contents_fts(docid, words) values(?, ?)`,
         docID,
         strings.Join(seg, " "),
	)

	if err != nil {
		return err
	}
	return nil
}

func main() {
	db, err := setupDB("database.sqlite")
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	listURL := "https://www.aozora.gr.jp/index_pages/person879.html"

	entries,err := findEntries(listURL)
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("found %d entries",len(entries))
	for _,entry := range entries {
		log.Printf("adding %+v\n",entry)
		content,err := extractText(entry.ZipURL)

		if err != nil {
			log.Println(err)
			continue
		}
		err = addEntry(db,&entry,content)
		if err != nil {
			log.Println(err)
			continue
		}
	}
}
