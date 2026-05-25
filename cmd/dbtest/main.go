package main

import (
	"database/sql"
	"log"
	"strings"
)

func setupDB(dsn string)(*sql.DB,error){
	db,err := sql.Open("sqlite3",dsn)
	if err != nil {
		return nil,err
	}

	queries := []string {
		`CREATE TABLE IF NOT EXISTS authors(
		author_id TEXT,
		 author TEXT,
		 PRIMARY KEY(author_id))`,
	  `CREATE TABLE IF NOT EXISTS contents(
		author_id TEXT,
		title_id TEXT,
		titleTEXT,
		content TEXT,
		PRIMARY KEY (author_id, title_id))`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS contents_fts USING fts4(words)`,
	}
	for _,query := range queries {
		_,err = db.Exec(query)
		if err != nil {
			return nil,err
		}
	}
	return db,nil

}

func addEntry(db *sql.DB,entry *Entry,content string)error{
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
	docID,err := res.LastInsertId()
	if err != nil {
		return err
	}
	t,err := tokenizer.New(ipa.Dict(),tokenizer.OmitBosEos())
	if err != nil {
		return err
	}
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
