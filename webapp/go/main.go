package main

import (
	crand "crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/gorilla/sessions"
	"github.com/jmoiron/sqlx"
	"goji.io"
	"goji.io/pat"
	"golang.org/x/crypto/bcrypt"
)

const (
	sessionName = "session_isucari"

	DefaultPaymentServiceURL  = "http://localhost:5555"
	DefaultShipmentServiceURL = "http://localhost:7000"

	ItemMinPrice    = 100
	ItemMaxPrice    = 1000000
	ItemPriceErrMsg = "商品価格は100ｲｽｺｲﾝ以上、1,000,000ｲｽｺｲﾝ以下にしてください"

	ItemStatusOnSale  = "on_sale"
	ItemStatusTrading = "trading"
	ItemStatusSoldOut = "sold_out"
	//ItemStatusStop    = "stop"
	//ItemStatusCancel  = "cancel"

	PaymentServiceIsucariAPIKey = "a15400e46c83635eb181-946abb51ff26a868317c"
	PaymentServiceIsucariShopID = "11"

	TransactionEvidenceStatusWaitShipping = "wait_shipping"
	TransactionEvidenceStatusWaitDone     = "wait_done"
	TransactionEvidenceStatusDone         = "done"

	ShippingsStatusInitial    = "initial"
	ShippingsStatusWaitPickup = "wait_pickup"
	ShippingsStatusShipping   = "shipping"
	ShippingsStatusDone       = "done"

	BumpChargeSeconds = 3 * time.Second

	ItemsPerPage        = 48
	TransactionsPerPage = 10

	BcryptCost = bcrypt.MinCost

	MaxCategoryID = 66
)

var (
	templates          *template.Template
	dbx                *sqlx.DB
	store              sessions.Store
	categories         [MaxCategoryID + 1]Category
	paymentServiceUrl  string
	shipmentServiceUrl string
	mapItemID          map[int64]int64
	mapShipID          map[int64]int64
)

type Config struct {
	Name string `json:"name" db:"name"`
	Val  string `json:"val" db:"val"`
}

type User struct {
	ID             int64     `json:"id" db:"id"`
	AccountName    string    `json:"account_name" db:"account_name"`
	HashedPassword []byte    `json:"-" db:"hashed_password"`
	Address        string    `json:"address,omitempty" db:"address"`
	NumSellItems   int       `json:"num_sell_items" db:"num_sell_items"`
	LastBump       time.Time `json:"-" db:"last_bump"`
	CreatedAt      time.Time `json:"-" db:"created_at"`
}

type UserSimple struct {
	ID           int64  `json:"id"`
	AccountName  string `json:"account_name"`
	NumSellItems int    `json:"num_sell_items"`
}

type Item struct {
	ID          int64     `json:"id" db:"id"`
	SellerID    int64     `json:"seller_id" db:"seller_id"`
	BuyerID     int64     `json:"buyer_id" db:"buyer_id"`
	Status      string    `json:"status" db:"status"`
	Name        string    `json:"name" db:"name"`
	Price       int       `json:"price" db:"price"`
	Description string    `json:"description" db:"description"`
	ImageName   string    `json:"image_name" db:"image_name"`
	CategoryID  int       `json:"category_id" db:"category_id"`
	CreatedAt   time.Time `json:"-" db:"created_at"`
	UpdatedAt   time.Time `json:"-" db:"updated_at"`
}

type ItemSimple struct {
	ID         int64       `json:"id"`
	SellerID   int64       `json:"seller_id"`
	Seller     *UserSimple `json:"seller"`
	Status     string      `json:"status"`
	Name       string      `json:"name"`
	Price      int         `json:"price"`
	ImageURL   string      `json:"image_url"`
	CategoryID int         `json:"category_id"`
	Category   *Category   `json:"category"`
	CreatedAt  int64       `json:"created_at"`
}

type ItemDetail struct {
	ID                        int64       `json:"id"`
	SellerID                  int64       `json:"seller_id"`
	Seller                    *UserSimple `json:"seller"`
	BuyerID                   int64       `json:"buyer_id,omitempty"`
	Buyer                     *UserSimple `json:"buyer,omitempty"`
	Status                    string      `json:"status"`
	Name                      string      `json:"name"`
	Price                     int         `json:"price"`
	Description               string      `json:"description"`
	ImageURL                  string      `json:"image_url"`
	CategoryID                int         `json:"category_id"`
	Category                  *Category   `json:"category"`
	TransactionEvidenceID     int64       `json:"transaction_evidence_id,omitempty"`
	TransactionEvidenceStatus string      `json:"transaction_evidence_status,omitempty"`
	ShippingStatus            string      `json:"shipping_status,omitempty"`
	CreatedAt                 int64       `json:"created_at"`
}

type TransactionEvidence struct {
	ID       int64  `json:"id" db:"id"`
	SellerID int64  `json:"seller_id" db:"seller_id"`
	BuyerID  int64  `json:"buyer_id" db:"buyer_id"`
	Status   string `json:"status" db:"status"`
	ItemID   int64  `json:"item_id" db:"item_id"`
	//ItemName           string    `json:"item_name" db:"item_name"`
	ItemPrice int `json:"item_price" db:"item_price"`
	//ItemDescription    string    `json:"item_description" db:"item_description"`
	//ItemCategoryID     int       `json:"item_category_id" db:"item_category_id"`
	//ItemRootCategoryID int       `json:"item_root_category_id" db:"item_root_category_id"`
	CreatedAt time.Time `json:"-" db:"created_at"`
	UpdatedAt time.Time `json:"-" db:"updated_at"`
}

type Shipping struct {
	TransactionEvidenceID int64  `json:"transaction_evidence_id" db:"transaction_evidence_id"`
	Status                string `json:"status" db:"status"`
	//ItemName              string    `json:"item_name" db:"item_name"`
	ItemID    int64  `json:"item_id" db:"item_id"`
	ReserveID string `json:"reserve_id" db:"reserve_id"`
	//ReserveTime           int64     `json:"reserve_time" db:"reserve_time"`
	//ToAddress             string    `json:"to_address" db:"to_address"`
	//ToName                string    `json:"to_name" db:"to_name"`
	//FromAddress           string    `json:"from_address" db:"from_address"`
	//FromName              string    `json:"from_name" db:"from_name"`
	ImgBinary []byte    `json:"-" db:"img_binary"`
	CreatedAt time.Time `json:"-" db:"created_at"`
	UpdatedAt time.Time `json:"-" db:"updated_at"`
}

type Category struct {
	ID                 int    `json:"id" db:"id"`
	ParentID           int    `json:"parent_id" db:"parent_id"`
	CategoryName       string `json:"category_name" db:"category_name"`
	ParentCategoryName string `json:"parent_category_name,omitempty" db:"-"`
}

type reqInitialize struct {
	PaymentServiceURL  string `json:"payment_service_url"`
	ShipmentServiceURL string `json:"shipment_service_url"`
}

type resInitialize struct {
	Campaign int    `json:"campaign"`
	Language string `json:"language"`
}

type resNewItems struct {
	RootCategoryID   int          `json:"root_category_id,omitempty"`
	RootCategoryName string       `json:"root_category_name,omitempty"`
	HasNext          bool         `json:"has_next"`
	Items            []ItemSimple `json:"items"`
}

type resUserItems struct {
	User    *UserSimple  `json:"user"`
	HasNext bool         `json:"has_next"`
	Items   []ItemSimple `json:"items"`
}

type resTransactions struct {
	HasNext bool         `json:"has_next"`
	Items   []ItemDetail `json:"items"`
}

type reqRegister struct {
	AccountName string `json:"account_name"`
	Address     string `json:"address"`
	Password    string `json:"password"`
}

type reqLogin struct {
	AccountName string `json:"account_name"`
	Password    string `json:"password"`
}

type reqItemEdit struct {
	CSRFToken string `json:"csrf_token"`
	ItemID    int64  `json:"item_id"`
	ItemPrice int    `json:"item_price"`
}

type resItemEdit struct {
	ItemID        int64 `json:"item_id"`
	ItemPrice     int   `json:"item_price"`
	ItemCreatedAt int64 `json:"item_created_at"`
	ItemUpdatedAt int64 `json:"item_updated_at"`
}

type reqBuy struct {
	CSRFToken string `json:"csrf_token"`
	ItemID    int64  `json:"item_id"`
	Token     string `json:"token"`
}

type resBuy struct {
	TransactionEvidenceID int64 `json:"transaction_evidence_id"`
}

type resSell struct {
	ID int64 `json:"id"`
}

type reqPostShip struct {
	CSRFToken string `json:"csrf_token"`
	ItemID    int64  `json:"item_id"`
}

type resPostShip struct {
	Path      string `json:"path"`
	ReserveID string `json:"reserve_id"`
}

type reqPostShipDone struct {
	CSRFToken string `json:"csrf_token"`
	ItemID    int64  `json:"item_id"`
}

type reqPostComplete struct {
	CSRFToken string `json:"csrf_token"`
	ItemID    int64  `json:"item_id"`
}

type reqBump struct {
	CSRFToken string `json:"csrf_token"`
	ItemID    int64  `json:"item_id"`
}

type resSetting struct {
	CSRFToken         string                      `json:"csrf_token"`
	PaymentServiceURL string                      `json:"payment_service_url"`
	User              *User                       `json:"user,omitempty"`
	Categories        [MaxCategoryID + 1]Category `json:"categories"`
}

func init() {
	store = sessions.NewCookieStore([]byte("abc"))

	log.SetFlags(log.Ldate | log.Ltime | log.Lshortfile)

	templates = template.Must(template.ParseFiles(
		"../public/index.html",
	))

	categories = initCategories()
	paymentServiceUrl = DefaultPaymentServiceURL
	shipmentServiceUrl = DefaultShipmentServiceURL
	mapItemID = map[int64]int64{}
	mapShipID = map[int64]int64{}
}

func initCategories() [MaxCategoryID + 1]Category {
	embed := [...]Category{
		Category{1, 0, "ソファー", ""},
		Category{2, 1, "一人掛けソファー", "ソファー"},
		Category{3, 1, "二人掛けソファー", "ソファー"},
		Category{4, 1, "コーナーソファー", "ソファー"},
		Category{5, 1, "二段ソファー", "ソファー"},
		Category{6, 1, "ソファーベッド", "ソファー"},
		Category{10, 0, "家庭用チェア", ""},
		Category{11, 10, "スツール", "家庭用チェア"},
		Category{12, 10, "クッションスツール", "家庭用チェア"},
		Category{13, 10, "ダイニングチェア", "家庭用チェア"},
		Category{14, 10, "リビングチェア", "家庭用チェア"},
		Category{15, 10, "カウンターチェア", "家庭用チェア"},
		Category{20, 0, "キッズチェア", ""},
		Category{21, 20, "学習チェア", "キッズチェア"},
		Category{22, 20, "ベビーソファ", "キッズチェア"},
		Category{23, 20, "キッズハイチェア", "キッズチェア"},
		Category{24, 20, "テーブルチェア", "キッズチェア"},
		Category{30, 0, "オフィスチェア", ""},
		Category{31, 30, "デスクチェア", "オフィスチェア"},
		Category{32, 30, "ビジネスチェア", "オフィスチェア"},
		Category{33, 30, "回転チェア", "オフィスチェア"},
		Category{34, 30, "リクライニングチェア", "オフィスチェア"},
		Category{35, 30, "投擲用椅子", "オフィスチェア"},
		Category{40, 0, "折りたたみ椅子", ""},
		Category{41, 40, "パイプ椅子", "折りたたみ椅子"},
		Category{42, 40, "木製折りたたみ椅子", "折りたたみ椅子"},
		Category{43, 40, "キッチンチェア", "折りたたみ椅子"},
		Category{44, 40, "アウトドアチェア", "折りたたみ椅子"},
		Category{45, 40, "作業椅子", "折りたたみ椅子"},
		Category{50, 0, "ベンチ", ""},
		Category{51, 50, "一人掛けベンチ", "ベンチ"},
		Category{52, 50, "二人掛けベンチ", "ベンチ"},
		Category{53, 50, "アウトドア用ベンチ", "ベンチ"},
		Category{54, 50, "収納付きベンチ", "ベンチ"},
		Category{55, 50, "背もたれ付きベンチ", "ベンチ"},
		Category{56, 50, "ベンチマーク", "ベンチ"},
		Category{60, 0, "座椅子", ""},
		Category{61, 60, "和風座椅子", "座椅子"},
		Category{62, 60, "高座椅子", "座椅子"},
		Category{63, 60, "ゲーミング座椅子", "座椅子"},
		Category{64, 60, "ロッキングチェア", "座椅子"},
		Category{65, 60, "座布団", "座椅子"},
		Category{66, 60, "空気椅子", "座椅子"},
	}

	cs := [MaxCategoryID + 1]Category{}
	for _, v := range embed {
		cs[v.ID] = v
	}

	return cs
}

func initMapItemID() error {
	mapItemID = map[int64]int64{}

	rows, err := dbx.Query("SELECT `id`, `seller_id` FROM `items`")
	if err != nil {
		return err
	}

	for rows.Next() {
		var itemID, sellerID int64
		if err = rows.Scan(&itemID, &sellerID); err != nil {
			return err
		}
		mapItemID[itemID] = sellerID
	}
	return nil
}

func main() {
	host := os.Getenv("MYSQL_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := os.Getenv("MYSQL_PORT")
	if port == "" {
		port = "3306"
	}
	_, err := strconv.Atoi(port)
	if err != nil {
		log.Fatalf("failed to read DB port number from an environment variable MYSQL_PORT.\nError: %s", err.Error())
	}
	user := os.Getenv("MYSQL_USER")
	if user == "" {
		user = "isucari"
	}
	dbname := os.Getenv("MYSQL_DBNAME")
	if dbname == "" {
		dbname = "isucari"
	}
	password := os.Getenv("MYSQL_PASS")
	if password == "" {
		password = "isucari"
	}

	dsn := fmt.Sprintf(
		"%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=true&loc=Local",
		user,
		password,
		host,
		port,
		dbname,
	)

	dbx, err = sqlx.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("failed to connect to DB: %s.", err.Error())
	}
	defer dbx.Close()

	mux := goji.NewMux()

	// API
	mux.HandleFunc(pat.Post("/initialize"), postInitialize)
	mux.HandleFunc(pat.Get("/new_items.json"), getNewItems)
	mux.HandleFunc(pat.Get("/new_items/:root_category_id.json"), getNewCategoryItems)
	mux.HandleFunc(pat.Get("/users/transactions.json"), getTransactions)
	mux.HandleFunc(pat.Get("/users/:user_id.json"), getUserItems)
	mux.HandleFunc(pat.Get("/items/:item_id.json"), getItem)
	mux.HandleFunc(pat.Post("/items/edit"), postItemEdit)
	mux.HandleFunc(pat.Post("/buy"), postBuy)
	mux.HandleFunc(pat.Post("/sell"), postSell)
	mux.HandleFunc(pat.Post("/ship"), postShip)
	mux.HandleFunc(pat.Post("/ship_done"), postShipDone)
	mux.HandleFunc(pat.Post("/complete"), postComplete)
	mux.HandleFunc(pat.Get("/transactions/:transaction_evidence_id.png"), getQRCode)
	mux.HandleFunc(pat.Post("/bump"), postBump)
	mux.HandleFunc(pat.Get("/settings"), getSettings)
	mux.HandleFunc(pat.Post("/login"), postLogin)
	mux.HandleFunc(pat.Post("/register"), postRegister)
	mux.HandleFunc(pat.Get("/reports.json"), getReports)
	// Frontend
	mux.HandleFunc(pat.Get("/"), getIndex)
	mux.HandleFunc(pat.Get("/login"), getIndex)
	mux.HandleFunc(pat.Get("/register"), getIndex)
	mux.HandleFunc(pat.Get("/timeline"), getIndex)
	mux.HandleFunc(pat.Get("/categories/:category_id/items"), getIndex)
	mux.HandleFunc(pat.Get("/sell"), getIndex)
	mux.HandleFunc(pat.Get("/items/:item_id"), getIndex)
	mux.HandleFunc(pat.Get("/items/:item_id/edit"), getIndex)
	mux.HandleFunc(pat.Get("/items/:item_id/buy"), getIndex)
	mux.HandleFunc(pat.Get("/buy/complete"), getIndex)
	mux.HandleFunc(pat.Get("/transactions/:transaction_id"), getIndex)
	mux.HandleFunc(pat.Get("/users/:user_id"), getIndex)
	mux.HandleFunc(pat.Get("/users/setting"), getIndex)
	// Assets
	mux.Handle(pat.Get("/*"), http.FileServer(http.Dir("../public")))
	log.Fatal(http.ListenAndServe(":8000", mux))
}

func getSession(r *http.Request) *sessions.Session {
	session, _ := store.Get(r, sessionName)

	return session
}

func getUserAndCSRToken(r *http.Request) (user User, token string, errCode int) {
	session := getSession(r)

	csrfToken, ok := session.Values["csrf_token"]
	if ok {
		token = csrfToken.(string)
	}

	userID, ok := session.Values["user_id"]
	if !ok {
		errCode = http.StatusNotFound
		return
	}

	err := dbx.Get(&user, "SELECT * FROM `users` WHERE `id` = ?", userID)
	if err == sql.ErrNoRows {
		errCode = http.StatusNotFound
		return
	}
	if err != nil {
		log.Print(err)
		errCode = http.StatusInternalServerError
		return
	}

	errCode = http.StatusOK
	return
}

func getUser(r *http.Request) (user User, errCode int, errMsg string) {
	session := getSession(r)
	userID, ok := session.Values["user_id"]
	if !ok {
		return user, http.StatusNotFound, "no session"
	}

	err := dbx.Get(&user, "SELECT * FROM `users` WHERE `id` = ?", userID)
	if err == sql.ErrNoRows {
		return user, http.StatusNotFound, "user not found"
	}
	if err != nil {
		log.Print(err)
		return user, http.StatusInternalServerError, "db error"
	}

	return user, http.StatusOK, ""
}

func getUserID(r *http.Request) (userID int64, ok bool) {
	session := getSession(r)

	val, ok := session.Values["user_id"]
	if !ok {
		return userID, false
	}

	userID, ok = val.(int64)
	if !ok {
		return userID, false
	}

	return
}

func getUserSimpleByID(q sqlx.Queryer, userID int64) (userSimple UserSimple, err error) {
	user := User{}
	err = sqlx.Get(q, &user, "SELECT * FROM `users` WHERE `id` = ?", userID)
	if err != nil {
		return userSimple, err
	}
	userSimple.ID = user.ID
	userSimple.AccountName = user.AccountName
	userSimple.NumSellItems = user.NumSellItems
	return userSimple, err
}

func getSellersByItems(items []Item) (mapSeller map[int64]UserSimple, err error) {
	mapSellerID := map[int64]bool{}
	for _, item := range items {
		mapSellerID[item.SellerID] = true
	}
	sellerIDs := []int64{}
	for k, _ := range mapSellerID {
		sellerIDs = append(sellerIDs, k)
	}

	return getUsersByIDs(sellerIDs)
}

func getBuyersByItems(items []Item) (mapBuyer map[int64]UserSimple, err error) {
	mapBuyerID := map[int64]bool{}
	for _, item := range items {
		mapBuyerID[item.BuyerID] = true
	}
	buyerIDs := []int64{}
	for k, _ := range mapBuyerID {
		buyerIDs = append(buyerIDs, k)
	}
	return getUsersByIDs(buyerIDs)
}

func getUsersByItems(items []Item) (mapUser map[int64]UserSimple, err error) {
	mapUserID := map[int64]bool{}
	for _, item := range items {
		mapUserID[item.SellerID] = true
		mapUserID[item.BuyerID] = true
	}
	userIDs := []int64{}
	for k, _ := range mapUserID {
		userIDs = append(userIDs, k)
	}
	return getUsersByIDs(userIDs)
}

func getUsersByIDs(ids []int64) (mapUser map[int64]UserSimple, err error) {
	inQuery, inArgs, err := sqlx.In("SELECT * FROM `users` WHERE `id` IN (?)", ids)
	if err != nil {
		log.Printf("seller not found: %v", err)
		return mapUser, err
	}

	var users []User
	err = dbx.Select(&users, inQuery, inArgs...)
	if err != nil {
		log.Printf("seller not found: %v", err)
		return mapUser, err
	}

	mapUser = map[int64]UserSimple{}
	for _, user := range users {
		mapUser[user.ID] = UserSimple{user.ID, user.AccountName, user.NumSellItems}
	}
	return mapUser, nil
}

func getCategoryByID(categoryID int) (category Category, err error) {
	if 0 < categoryID && categoryID <= MaxCategoryID {
		category = categories[categoryID]
		if category.ID != 0 {
			return category, nil
		}
	}
	log.Printf("category not found: %d", categoryID)
	return category, fmt.Errorf("category not found: %d", categoryID)
}

func getConfigByName(name string) (string, error) {
	config := Config{}
	err := dbx.Get(&config, "SELECT * FROM `configs` WHERE `name` = ?", name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	if err != nil {
		log.Print(err)
		return "", err
	}
	return config.Val, err
}

func getPaymentServiceURL() string {
	return paymentServiceUrl
}

func getShipmentServiceURL() string {
	return shipmentServiceUrl
}

func getIndex(w http.ResponseWriter, r *http.Request) {
	templates.ExecuteTemplate(w, "index.html", struct{}{})
}

func postInitialize(w http.ResponseWriter, r *http.Request) {
	ri := reqInitialize{}

	err := json.NewDecoder(r.Body).Decode(&ri)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	cmd := exec.Command("../sql/init.sh")
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stderr
	cmd.Run()
	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "exec init.sh error")
		return
	}

	err = initMapItemID()
	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "initialize mapItemID error")
		return
	}

	mapShipID = map[int64]int64{}

	_, err = dbx.Exec(
		"INSERT INTO `configs` (`name`, `val`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `val` = VALUES(`val`)",
		"payment_service_url",
		ri.PaymentServiceURL,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}
	paymentServiceUrl = ri.PaymentServiceURL

	_, err = dbx.Exec(
		"INSERT INTO `configs` (`name`, `val`) VALUES (?, ?) ON DUPLICATE KEY UPDATE `val` = VALUES(`val`)",
		"shipment_service_url",
		ri.ShipmentServiceURL,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}
	shipmentServiceUrl = ri.ShipmentServiceURL

	res := resInitialize{
		// キャンペーン実施時には還元率の設定返す。詳しくはマニュアルを参照のこと。
		Campaign: 0,
		// 実装言語を返す
		Language: "Go",
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(res)
}

func getNewItems(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	itemIDStr := query.Get("item_id")
	var itemID int64
	var err error
	if itemIDStr != "" {
		itemID, err = strconv.ParseInt(itemIDStr, 10, 64)
		if err != nil || itemID <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "item_id param error")
			return
		}
	}

	createdAtStr := query.Get("created_at")
	var createdAt int64
	if createdAtStr != "" {
		createdAt, err = strconv.ParseInt(createdAtStr, 10, 64)
		if err != nil || createdAt <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "created_at param error")
			return
		}
	}

	items := []Item{}
	if itemID > 0 && createdAt > 0 {
		// paging
		err := dbx.Select(&items,
			"SELECT * FROM `items` WHERE `status` IN (?,?) AND (`created_at` < ?  OR (`created_at` <= ? AND `id` < ?)) ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			ItemStatusOnSale,
			ItemStatusSoldOut,
			time.Unix(createdAt, 0),
			time.Unix(createdAt, 0),
			itemID,
			ItemsPerPage+1,
		)
		if err != nil {
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}
	} else {
		// 1st page
		err := dbx.Select(&items,
			"SELECT * FROM `items` WHERE `status` IN (?,?) ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			ItemStatusOnSale,
			ItemStatusSoldOut,
			ItemsPerPage+1,
		)
		if err != nil {
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	mapSeller, err := getSellersByItems(items)
	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	itemSimples := []ItemSimple{}
	for _, item := range items {
		seller, ok := mapSeller[item.SellerID]
		if !ok {
			outputErrorMsg(w, http.StatusNotFound, "seller not found")
			return
		}
		category, err := getCategoryByID(item.CategoryID)
		if err != nil {
			outputErrorMsg(w, http.StatusNotFound, "category not found")
			return
		}
		itemSimples = append(itemSimples, ItemSimple{
			ID:         item.ID,
			SellerID:   item.SellerID,
			Seller:     &seller,
			Status:     item.Status,
			Name:       item.Name,
			Price:      item.Price,
			ImageURL:   getImageURL(item.ImageName),
			CategoryID: item.CategoryID,
			Category:   &category,
			CreatedAt:  item.CreatedAt.Unix(),
		})
	}

	hasNext := false
	if len(itemSimples) > ItemsPerPage {
		hasNext = true
		itemSimples = itemSimples[0:ItemsPerPage]
	}

	rni := resNewItems{
		Items:   itemSimples,
		HasNext: hasNext,
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(rni)
}

func getNewCategoryItems(w http.ResponseWriter, r *http.Request) {
	rootCategoryIDStr := pat.Param(r, "root_category_id")
	rootCategoryID, err := strconv.Atoi(rootCategoryIDStr)
	if err != nil || rootCategoryID <= 0 {
		outputErrorMsg(w, http.StatusBadRequest, "incorrect category id")
		return
	}

	rootCategory, err := getCategoryByID(rootCategoryID)
	if err != nil || rootCategory.ParentID != 0 {
		outputErrorMsg(w, http.StatusNotFound, "category not found")
		return
	}

	categoryIDs := []int{}
	for _, category := range categories {
		if category.ParentID == rootCategory.ID {
			categoryIDs = append(categoryIDs, category.ID)
		}
	}

	query := r.URL.Query()
	itemIDStr := query.Get("item_id")
	var itemID int64
	if itemIDStr != "" {
		itemID, err = strconv.ParseInt(itemIDStr, 10, 64)
		if err != nil || itemID <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "item_id param error")
			return
		}
	}

	createdAtStr := query.Get("created_at")
	var createdAt int64
	if createdAtStr != "" {
		createdAt, err = strconv.ParseInt(createdAtStr, 10, 64)
		if err != nil || createdAt <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "created_at param error")
			return
		}
	}

	var inQuery string
	var inArgs []interface{}
	if itemID > 0 && createdAt > 0 {
		// paging
		inQuery, inArgs, err = sqlx.In(
			"SELECT * FROM `items` WHERE `status` IN (?,?) AND category_id IN (?) AND (`created_at` < ?  OR (`created_at` <= ? AND `id` < ?)) ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			ItemStatusOnSale,
			ItemStatusSoldOut,
			categoryIDs,
			time.Unix(createdAt, 0),
			time.Unix(createdAt, 0),
			itemID,
			ItemsPerPage+1,
		)
		if err != nil {
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}
	} else {
		// 1st page
		inQuery, inArgs, err = sqlx.In(
			"SELECT * FROM `items` WHERE `status` IN (?,?) AND category_id IN (?) ORDER BY created_at DESC, id DESC LIMIT ?",
			ItemStatusOnSale,
			ItemStatusSoldOut,
			categoryIDs,
			ItemsPerPage+1,
		)
		if err != nil {
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	items := []Item{}
	err = dbx.Select(&items, inQuery, inArgs...)

	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	mapSeller, err := getSellersByItems(items)
	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	itemSimples := []ItemSimple{}
	for _, item := range items {
		seller, ok := mapSeller[item.SellerID]
		if !ok {
			outputErrorMsg(w, http.StatusNotFound, "seller not found")
			return
		}
		category, err := getCategoryByID(item.CategoryID)
		if err != nil {
			outputErrorMsg(w, http.StatusNotFound, "category not found")
			return
		}
		itemSimples = append(itemSimples, ItemSimple{
			ID:         item.ID,
			SellerID:   item.SellerID,
			Seller:     &seller,
			Status:     item.Status,
			Name:       item.Name,
			Price:      item.Price,
			ImageURL:   getImageURL(item.ImageName),
			CategoryID: item.CategoryID,
			Category:   &category,
			CreatedAt:  item.CreatedAt.Unix(),
		})
	}

	hasNext := false
	if len(itemSimples) > ItemsPerPage {
		hasNext = true
		itemSimples = itemSimples[0:ItemsPerPage]
	}

	rni := resNewItems{
		RootCategoryID:   rootCategory.ID,
		RootCategoryName: rootCategory.CategoryName,
		Items:            itemSimples,
		HasNext:          hasNext,
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(rni)

}

func getUserItems(w http.ResponseWriter, r *http.Request) {
	userIDStr := pat.Param(r, "user_id")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil || userID <= 0 {
		outputErrorMsg(w, http.StatusBadRequest, "incorrect user id")
		return
	}

	userSimple, err := getUserSimpleByID(dbx, userID)
	if err != nil {
		outputErrorMsg(w, http.StatusNotFound, "user not found")
		return
	}

	query := r.URL.Query()
	itemIDStr := query.Get("item_id")
	var itemID int64
	if itemIDStr != "" {
		itemID, err = strconv.ParseInt(itemIDStr, 10, 64)
		if err != nil || itemID <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "item_id param error")
			return
		}
	}

	createdAtStr := query.Get("created_at")
	var createdAt int64
	if createdAtStr != "" {
		createdAt, err = strconv.ParseInt(createdAtStr, 10, 64)
		if err != nil || createdAt <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "created_at param error")
			return
		}
	}

	items := []Item{}
	if itemID > 0 && createdAt > 0 {
		// paging
		err := dbx.Select(&items,
			"SELECT * FROM `items` WHERE `seller_id` = ? AND (`created_at` < ?  OR (`created_at` <= ? AND `id` < ?)) ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			userSimple.ID,
			time.Unix(createdAt, 0),
			time.Unix(createdAt, 0),
			itemID,
			ItemsPerPage+1,
		)
		if err != nil {
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}
	} else {
		// 1st page
		err := dbx.Select(&items,
			"SELECT * FROM `items` WHERE `seller_id` = ? ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			userSimple.ID,
			ItemsPerPage+1,
		)
		if err != nil {
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}
	}

	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	itemSimples := []ItemSimple{}
	for _, item := range items {
		category, err := getCategoryByID(item.CategoryID)
		if err != nil {
			outputErrorMsg(w, http.StatusNotFound, "category not found")
			return
		}
		itemSimples = append(itemSimples, ItemSimple{
			ID:         item.ID,
			SellerID:   item.SellerID,
			Seller:     &userSimple,
			Status:     item.Status,
			Name:       item.Name,
			Price:      item.Price,
			ImageURL:   getImageURL(item.ImageName),
			CategoryID: item.CategoryID,
			Category:   &category,
			CreatedAt:  item.CreatedAt.Unix(),
		})
	}

	hasNext := false
	if len(itemSimples) > ItemsPerPage {
		hasNext = true
		itemSimples = itemSimples[0:ItemsPerPage]
	}

	rui := resUserItems{
		User:    &userSimple,
		Items:   itemSimples,
		HasNext: hasNext,
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(rui)
}

func getNewItemsByUserID(userID, itemID, createdAt int64) (items []Item, err error) {
	var rows *sql.Rows
	if itemID > 0 && createdAt > 0 {
		rows, err = dbx.Query(
			"(SELECT `id`, `seller_id`, `buyer_id`, `created_at` FROM `items` WHERE `seller_id` = ? AND (`created_at` < ? OR (`created_at` <= ? AND `id` < ?))) UNION (SELECT `id`, `seller_id`, `buyer_id`, `created_at` FROM `items` WHERE `buyer_id` = ? AND (`created_at` < ? OR (`created_at` <= ? AND `id` < ?))) ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			userID,
			time.Unix(createdAt, 0),
			time.Unix(createdAt, 0),
			itemID,
			userID,
			time.Unix(createdAt, 0),
			time.Unix(createdAt, 0),
			itemID,
			TransactionsPerPage+1,
		)
	} else {
		// 1st page
		rows, err = dbx.Query(
			"(SELECT `id`, `seller_id`, `buyer_id`, `created_at` FROM `items` WHERE `seller_id` = ?) UNION (SELECT `id`, `seller_id`, `buyer_id`, `created_at` FROM `items` WHERE `buyer_id` = ?) ORDER BY `created_at` DESC, `id` DESC LIMIT ?",
			userID,
			userID,
			TransactionsPerPage+1,
		)
	}

	if err != nil {
		return
	}

	for rows.Next() {
		var itemID, sellerID, buyerID int64
		var createdAt time.Time
		if err = rows.Scan(&itemID, &sellerID, &buyerID, &createdAt); err != nil {
			log.Print(err)
			rows.Close()
			return
		}
		items = append(items, Item{ID: itemID, SellerID: sellerID, BuyerID: buyerID, CreatedAt: createdAt})
	}
	return
}

func getTransactions(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	itemIDStr := query.Get("item_id")
	var err error
	var itemID int64
	if itemIDStr != "" {
		itemID, err = strconv.ParseInt(itemIDStr, 10, 64)
		if err != nil || itemID <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "item_id param error")
			return
		}
	}

	createdAtStr := query.Get("created_at")
	var createdAt int64
	if createdAtStr != "" {
		createdAt, err = strconv.ParseInt(createdAtStr, 10, 64)
		if err != nil || createdAt <= 0 {
			outputErrorMsg(w, http.StatusBadRequest, "created_at param error")
			return
		}
	}

	userID, ok := getUserID(r)
	if !ok {
		outputErrorMsg(w, http.StatusNotFound, "created_at param error")
		return
	}

	// only id, seller_id, buyer_id
	dumpItems, err := getNewItemsByUserID(userID, itemID, createdAt)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "get new items error")
		return
	}

	mapUsersCh := make(chan map[int64]UserSimple)
	go func() {
		mapUsers, err := getUsersByItems(dumpItems)
		if err != nil {
			mapUsersCh <- map[int64]UserSimple{}
		} else {
			mapUsersCh <- mapUsers
		}
	}()

	itemIDs := []int64{}
	transactionIDs := []int64{}
	mapRevShipID := map[int64]int64{}
	for _, item := range dumpItems {
		itemIDs = append(itemIDs, item.ID)
		if transactionID, ok := mapShipID[item.ID]; ok {
			mapRevShipID[transactionID] = item.ID
			transactionIDs = append(transactionIDs, transactionID)
		}
	}

	mapShipStatCh := make(chan map[int64]string)
	if len(transactionIDs) != 0 {
		go func() {
			mapShippingStatus := map[int64]string{}
			inQuery, inArgs, err := sqlx.In("SELECT `transaction_evidence_id`, `status` FROM `shippings` WHERE `transaction_evidence_id` IN (?)", transactionIDs)
			if err != nil {
				log.Print(err)
				mapShipStatCh <- map[int64]string{}
				return
			}

			rows, err := dbx.Query(inQuery, inArgs...)
			if err != nil {
				log.Print(err)
				mapShipStatCh <- map[int64]string{}
				return
			}
			for rows.Next() {
				var transactionEvidenceID int64
				var status string
				if err = rows.Scan(&transactionEvidenceID, &status); err != nil {
					rows.Close()
					mapShipStatCh <- map[int64]string{}
					return
				}
				mapShippingStatus[mapRevShipID[transactionEvidenceID]] = status
			}
			mapShipStatCh <- mapShippingStatus
		}()
	}

	inQuery, inArgs, err := sqlx.In("SELECT * FROM `items` WHERE `id` IN (?) ORDER BY `created_at` DESC, `id` DESC", itemIDs)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "sql error")
		return
	}
	var items []Item
	if err = dbx.Select(&items, inQuery, inArgs...); err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "sql error")
		return
	}

	mapUsers := <-mapUsersCh
	if len(mapUsers) == 0 {
		outputErrorMsg(w, http.StatusNotFound, "user not found")
		return
	}

	mapShippingStatus := map[int64]string{}
	if len(transactionIDs) != 0 {
		if mapShippingStatus = <-mapShipStatCh; len(mapShippingStatus) == 0 {
			outputErrorMsg(w, http.StatusInternalServerError, "sql error")
			return
		}
	}

	httpStatusCh := make(chan int)
	wg := sync.WaitGroup{}

	itemDetails := make([]ItemDetail, len(items))
	for idx, item := range items {
		wg.Add(1)
		go func(idx int, item Item) {
			defer wg.Done()
			seller, ok := mapUsers[item.SellerID]
			if !ok {
				httpStatusCh <- http.StatusNotFound
				return
			}
			category, err := getCategoryByID(item.CategoryID)
			if err != nil {
				httpStatusCh <- http.StatusNotFound
				return
			}

			itemDetail := ItemDetail{
				ID:       item.ID,
				SellerID: item.SellerID,
				Seller:   &seller,
				// BuyerID
				// Buyer
				// Status:      item.Status,
				Name:        item.Name,
				Price:       item.Price,
				Description: item.Description,
				ImageURL:    getImageURL(item.ImageName),
				CategoryID:  item.CategoryID,
				// TransactionEvidenceID
				// TransactionEvidenceStatus
				// ShippingStatus
				Category:  &category,
				CreatedAt: item.CreatedAt.Unix(),
			}

			if item.BuyerID != 0 {
				buyer, ok := mapUsers[item.BuyerID]
				if !ok {
					httpStatusCh <- http.StatusNotFound
					return
				}
				itemDetail.BuyerID = item.BuyerID
				itemDetail.Buyer = &buyer
			}

			status, ok := mapShippingStatus[item.ID]
			if ok {
				itemDetail.TransactionEvidenceID = mapShipID[item.ID]
				switch status {
				case ShippingsStatusInitial, ShippingsStatusWaitPickup:
					itemDetail.Status = ItemStatusTrading
					itemDetail.TransactionEvidenceStatus = TransactionEvidenceStatusWaitShipping
				case ShippingsStatusShipping:
					itemDetail.Status = ItemStatusTrading
					itemDetail.TransactionEvidenceStatus = TransactionEvidenceStatusWaitDone
				case ShippingsStatusDone:
					if item.Status == ItemStatusSoldOut {
						itemDetail.Status = ItemStatusSoldOut
						itemDetail.TransactionEvidenceStatus = TransactionEvidenceStatusDone
					} else {
						itemDetail.Status = ItemStatusSoldOut
						itemDetail.TransactionEvidenceStatus = TransactionEvidenceStatusWaitDone
					}
				}
				itemDetail.ShippingStatus = status
			} else {
				itemDetail.Status = ItemStatusOnSale
			}

			itemDetails[idx] = itemDetail
		}(idx, item)
	}

	go func() {
		wg.Wait()
		httpStatusCh <- http.StatusOK
	}()

	if status := <-httpStatusCh; status != http.StatusOK {
		outputErrorMsg(w, status, "db error")
		return
	}

	hasNext := false
	if len(itemDetails) > TransactionsPerPage {
		hasNext = true
		itemDetails = itemDetails[0:TransactionsPerPage]
	}

	rts := resTransactions{
		Items:   itemDetails,
		HasNext: hasNext,
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(rts)

}

func getItem(w http.ResponseWriter, r *http.Request) {
	badHttpStatusCh := make(chan int)
	userIDCh := make(chan int64)

	// get user_id
	go func() {
		userID, ok := getUserID(r)
		if !ok {
			badHttpStatusCh <- http.StatusNotFound
		} else {
			userIDCh <- userID
		}
		return
	}()

	itemIDStr := pat.Param(r, "item_id")
	itemID, err := strconv.ParseInt(itemIDStr, 10, 64)
	if err != nil || itemID <= 0 {
		outputErrorMsg(w, http.StatusBadRequest, "incorrect item id")
		return
	}

	item := Item{}
	err = dbx.Get(&item, "SELECT * FROM `items` WHERE `id` = ?", itemID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "item not found")
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	category, err := getCategoryByID(item.CategoryID)
	if err != nil {
		outputErrorMsg(w, http.StatusNotFound, "category not found")
		return
	}

	mapUser, err := getUsersByIDs([]int64{item.SellerID, item.BuyerID})
	if err != nil {
		outputErrorMsg(w, http.StatusNotFound, "user not found")
		return
	}

	seller, ok := mapUser[item.SellerID]
	if !ok {
		outputErrorMsg(w, http.StatusNotFound, "seller not found")
		return
	}

	itemDetail := ItemDetail{
		ID:       item.ID,
		SellerID: item.SellerID,
		Seller:   &seller,
		// BuyerID
		// Buyer
		Status:      item.Status,
		Name:        item.Name,
		Price:       item.Price,
		Description: item.Description,
		ImageURL:    getImageURL(item.ImageName),
		CategoryID:  item.CategoryID,
		// TransactionEvidenceID
		// TransactionEvidenceStatus
		// ShippingStatus
		Category:  &category,
		CreatedAt: item.CreatedAt.Unix(),
	}

	var userID int64
	select {
	case userID = <-userIDCh:
	case badHttpStatusCh := <-badHttpStatusCh:
		outputErrorMsg(w, badHttpStatusCh, "no session")
		return
	}

	if (userID == item.SellerID || userID == item.BuyerID) && item.BuyerID != 0 {
		buyer, ok := mapUser[item.BuyerID]
		if !ok {
			outputErrorMsg(w, http.StatusNotFound, "buyer not found")
			return
		}
		itemDetail.BuyerID = item.BuyerID
		itemDetail.Buyer = &buyer

		transactionEvidence := TransactionEvidence{}
		err = dbx.Get(&transactionEvidence, "SELECT * FROM `transaction_evidences` WHERE `item_id` = ?", item.ID)
		if err != nil && err != sql.ErrNoRows {
			// It's able to ignore ErrNoRows
			log.Print(err)
			outputErrorMsg(w, http.StatusInternalServerError, "db error")
			return
		}

		if transactionEvidence.ID > 0 {
			var status string
			err = dbx.Get(&status, "SELECT `status` FROM `shippings` WHERE `transaction_evidence_id` = ?", transactionEvidence.ID)
			if err == sql.ErrNoRows {
				outputErrorMsg(w, http.StatusNotFound, "shipping not found")
				return
			}
			if err != nil {
				log.Print(err)
				outputErrorMsg(w, http.StatusInternalServerError, "db error")
				return
			}

			itemDetail.TransactionEvidenceID = transactionEvidence.ID
			itemDetail.TransactionEvidenceStatus = transactionEvidence.Status
			itemDetail.ShippingStatus = status
		}
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(itemDetail)
}

func postItemEdit(w http.ResponseWriter, r *http.Request) {
	rie := reqItemEdit{}
	err := json.NewDecoder(r.Body).Decode(&rie)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	itemID := rie.ItemID
	price := rie.ItemPrice

	seller, csrfToken, errCode := getUserAndCSRToken(r)
	if rie.CSRFToken != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	if price < ItemMinPrice || price > ItemMaxPrice {
		outputErrorMsg(w, http.StatusBadRequest, ItemPriceErrMsg)
		return
	}

	sellerID, ok := mapItemID[itemID]
	if !ok {
		outputErrorMsg(w, http.StatusNotFound, "item not found")
		return
	}

	if sellerID != seller.ID {
		outputErrorMsg(w, http.StatusForbidden, "自分の商品以外は編集できません")
		return
	}

	tx := dbx.MustBegin()
	targetItem := Item{}
	err = tx.Get(&targetItem, "SELECT * FROM `items` WHERE `id` = ? FOR UPDATE", itemID)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	if targetItem.Status != ItemStatusOnSale {
		outputErrorMsg(w, http.StatusForbidden, "販売中の商品以外編集できません")
		tx.Rollback()
		return
	}

	_, err = tx.Exec("UPDATE `items` SET `price` = ?, `updated_at` = ? WHERE `id` = ?",
		price,
		time.Now(),
		itemID,
	)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	err = tx.Get(&targetItem, "SELECT * FROM `items` WHERE `id` = ?", itemID)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	tx.Commit()

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(&resItemEdit{
		ItemID:        targetItem.ID,
		ItemPrice:     targetItem.Price,
		ItemCreatedAt: targetItem.CreatedAt.Unix(),
		ItemUpdatedAt: targetItem.UpdatedAt.Unix(),
	})
}

func getQRCode(w http.ResponseWriter, r *http.Request) {
	transactionEvidenceIDStr := pat.Param(r, "transaction_evidence_id")
	transactionEvidenceID, err := strconv.ParseInt(transactionEvidenceIDStr, 10, 64)
	if err != nil || transactionEvidenceID <= 0 {
		outputErrorMsg(w, http.StatusBadRequest, "incorrect transaction_evidence id")
		return
	}

	seller, errCode, errMsg := getUser(r)
	if errMsg != "" {
		outputErrorMsg(w, errCode, errMsg)
		return
	}

	transactionEvidence := TransactionEvidence{}
	err = dbx.Get(&transactionEvidence, "SELECT * FROM `transaction_evidences` WHERE `id` = ?", transactionEvidenceID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "transaction_evidences not found")
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	if transactionEvidence.SellerID != seller.ID {
		outputErrorMsg(w, http.StatusForbidden, "権限がありません")
		return
	}

	shipping := Shipping{}
	err = dbx.Get(&shipping, "SELECT * FROM `shippings` WHERE `transaction_evidence_id` = ?", transactionEvidence.ID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "shippings not found")
		return
	}
	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	if shipping.Status != ShippingsStatusWaitPickup && shipping.Status != ShippingsStatusShipping {
		outputErrorMsg(w, http.StatusForbidden, "qrcode not available")
		return
	}

	if len(shipping.ImgBinary) == 0 {
		outputErrorMsg(w, http.StatusInternalServerError, "empty qrcode image")
		return
	}

	w.Header().Set("Content-Type", "image/png")
	w.Write(shipping.ImgBinary)
}

func postBuy(w http.ResponseWriter, r *http.Request) {
	rb := reqBuy{}

	err := json.NewDecoder(r.Body).Decode(&rb)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	buyer, csrfToken, errCode := getUserAndCSRToken(r)
	if rb.CSRFToken != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	sellerID, ok := mapItemID[rb.ItemID]
	if !ok {
		outputErrorMsg(w, http.StatusNotFound, "item not found")
		return
	}

	if sellerID == buyer.ID {
		outputErrorMsg(w, http.StatusForbidden, "自分の商品は買えません")
		return
	}

	scrCh := make(chan *APIShipmentCreateRes)
	go func() {
		seller := User{}
		err = dbx.Get(&seller, "SELECT * FROM `users` WHERE `id` = ?", sellerID)
		if err != nil {
			log.Print(err)
			scrCh <- nil
			return
		}

		scr, err := APIShipmentCreate(getShipmentServiceURL(), &APIShipmentCreateReq{
			ToAddress:   buyer.Address,
			ToName:      buyer.AccountName,
			FromAddress: seller.Address,
			FromName:    seller.AccountName,
		})
		if err == nil {
			scrCh <- scr
		} else {
			log.Print(err)
			scrCh <- nil
		}
	}()

	tx := dbx.MustBegin()

	var targetItem Item
	err = tx.Get(&targetItem,
		"SELECT /*+ MAX_EXECUTION_TIME(5000) */ * FROM `items` WHERE `id` = ? AND `status` = ? FOR UPDATE",
		rb.ItemID,
		ItemStatusOnSale,
	)
	if err == sql.ErrNoRows || err.Error() == "Error 3024: Query execution was interrupted, maximum statement execution time exceeded" {
		outputErrorMsg(w, http.StatusForbidden, "item is not for sale")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	_, err = tx.Exec("UPDATE `items` SET `buyer_id` = ?, `status` = ?, `updated_at` = ? WHERE `id` = ? AND `status` = ? ",
		buyer.ID,
		ItemStatusTrading,
		time.Now(),
		rb.ItemID,
		ItemStatusOnSale,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	scr := <-scrCh
	if scr == nil {
		outputErrorMsg(w, http.StatusInternalServerError, "failed to request to shipment service")
		tx.Rollback()
		return
	}

	pstr, err := APIPaymentToken(getPaymentServiceURL(), &APIPaymentServiceTokenReq{
		ShopID: PaymentServiceIsucariShopID,
		Token:  rb.Token,
		APIKey: PaymentServiceIsucariAPIKey,
		Price:  targetItem.Price,
	})
	if err != nil {
		outputErrorMsg(w, http.StatusInternalServerError, "payment service is failed")
		tx.Rollback()
		return
	}
	if pstr.Status != "ok" {
		switch pstr.Status {
		case "invalid":
			outputErrorMsg(w, http.StatusBadRequest, "カード情報に誤りがあります")
		case "fail":
			outputErrorMsg(w, http.StatusBadRequest, "カードの残高が足りません")
		default:
			outputErrorMsg(w, http.StatusBadRequest, "想定外のエラー")
		}
		tx.Rollback()
		return
	}
	tx.Commit()

	result, err := dbx.Exec("INSERT INTO `transaction_evidences` (`seller_id`, `buyer_id`, `status`, `item_id`, `item_price`) VALUES (?, ?, ?, ?, ?)",
		targetItem.SellerID,
		buyer.ID,
		TransactionEvidenceStatusWaitShipping,
		targetItem.ID,
		targetItem.Price,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}
	transactionEvidenceID, err := result.LastInsertId()
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	_, err = dbx.Exec("INSERT INTO `shippings` (`transaction_evidence_id`, `status`, `item_id`, `reserve_id`, `img_binary`) VALUES (?,?,?,?,?)",
		transactionEvidenceID,
		ShippingsStatusInitial,
		targetItem.ID,
		scr.ReserveID,
		"",
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}
	mapShipID[targetItem.ID] = transactionEvidenceID

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(resBuy{TransactionEvidenceID: transactionEvidenceID})
}

func postShip(w http.ResponseWriter, r *http.Request) {
	reqps := reqPostShip{}

	err := json.NewDecoder(r.Body).Decode(&reqps)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	itemID := reqps.ItemID

	seller, csrfToken, errCode := getUserAndCSRToken(r)
	if reqps.CSRFToken != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	transactionEvidence := TransactionEvidence{}

	err = dbx.Get(&transactionEvidence, "SELECT * FROM `transaction_evidences` WHERE `item_id` = ?", itemID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "transaction_evidences not found")
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")

		return
	}
	if transactionEvidence.SellerID != seller.ID {
		outputErrorMsg(w, http.StatusForbidden, "権限がありません")
		return
	}

	tx := dbx.MustBegin()

	var reserveID string
	err = tx.Get(&reserveID, "SELECT `reserve_id` FROM `shippings` WHERE `transaction_evidence_id` = ? FOR UPDATE", transactionEvidence.ID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "shippings not found")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	img, err := APIShipmentRequest(getShipmentServiceURL(), &APIShipmentRequestReq{
		ReserveID: reserveID,
	})
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "failed to request to shipment service")
		tx.Rollback()

		return
	}

	_, err = tx.Exec("UPDATE `shippings` SET `status` = ?, `img_binary` = ?, `updated_at` = ? WHERE `transaction_evidence_id` = ?",
		ShippingsStatusWaitPickup,
		img,
		time.Now(),
		transactionEvidence.ID,
	)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	tx.Commit()

	rps := resPostShip{
		Path:      fmt.Sprintf("/transactions/%d.png", transactionEvidence.ID),
		ReserveID: reserveID,
	}
	json.NewEncoder(w).Encode(rps)
}

func postShipDone(w http.ResponseWriter, r *http.Request) {
	reqpsd := reqPostShipDone{}

	err := json.NewDecoder(r.Body).Decode(&reqpsd)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	itemID := reqpsd.ItemID

	seller, csrfToken, errCode := getUserAndCSRToken(r)
	if reqpsd.CSRFToken != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	sellerID, ok := mapItemID[itemID]
	if !ok {
		outputErrorMsg(w, http.StatusNotFound, "transaction_evidence not found")
		return
	}
	if sellerID != seller.ID {
		outputErrorMsg(w, http.StatusForbidden, "権限がありません")
		return
	}

	tx := dbx.MustBegin()

	var transactionEvidenceID int64
	err = tx.Get(&transactionEvidenceID, "SELECT `id` FROM `transaction_evidences` WHERE `item_id` = ? AND `status` = ? FOR UPDATE", itemID, TransactionEvidenceStatusWaitShipping)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusForbidden, "準備ができていません")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	var reserveID string
	err = tx.Get(&reserveID, "SELECT `reserve_id` FROM `shippings` WHERE `transaction_evidence_id` = ? FOR UPDATE", transactionEvidenceID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "shippings not found")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	ssrCh := make(chan *APIShipmentStatusRes)
	go func() {
		ssr, err := APIShipmentStatus(getShipmentServiceURL(), &APIShipmentStatusReq{
			ReserveID: reserveID,
		})
		if err != nil {
			log.Print(err)
			ssrCh <- nil
		} else {
			ssrCh <- ssr
		}
	}()

	_, err = tx.Exec("UPDATE `transaction_evidences` SET `status` = ?, `updated_at` = ? WHERE `id` = ?",
		TransactionEvidenceStatusWaitDone,
		time.Now(),
		transactionEvidenceID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	ssr := <-ssrCh
	if ssr == nil {
		outputErrorMsg(w, http.StatusInternalServerError, "failed to request to shipment service")
	}
	if !(ssr.Status == ShippingsStatusShipping || ssr.Status == ShippingsStatusDone) {
		outputErrorMsg(w, http.StatusForbidden, "shipment service側で配送中か配送完了になっていません")
		tx.Rollback()
		return
	}
	_, err = tx.Exec("UPDATE `shippings` SET `status` = ?, `updated_at` = ? WHERE `transaction_evidence_id` = ?",
		ssr.Status,
		time.Now(),
		transactionEvidenceID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	tx.Commit()

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(resBuy{TransactionEvidenceID: transactionEvidenceID})
}

func postComplete(w http.ResponseWriter, r *http.Request) {
	reqpc := reqPostComplete{}

	err := json.NewDecoder(r.Body).Decode(&reqpc)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	itemID := reqpc.ItemID

	buyer, csrfToken, errCode := getUserAndCSRToken(r)
	if reqpc.CSRFToken != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	var transactionEvidenceID int64
	err = dbx.Get(&transactionEvidenceID, "SELECT `id` FROM `transaction_evidences` WHERE `item_id` = ?",
		itemID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	ssrCh := make(chan *APIShipmentStatusRes)
	go func() {
		var reserveID string
		err = dbx.Get(&reserveID, "SELECT `reserve_id` FROM `shippings` WHERE `transaction_evidence_id` = ?", transactionEvidenceID)
		if err != nil {
			log.Print(err)
			ssrCh <- nil
			return
		}

		ssr, err := APIShipmentStatus(getShipmentServiceURL(), &APIShipmentStatusReq{
			ReserveID: reserveID,
		})
		if err != nil {
			log.Print(err)
			ssrCh <- nil
		} else {
			ssrCh <- ssr
		}
	}()

	tx := dbx.MustBegin()

	_, err = tx.Exec("UPDATE `transaction_evidences` SET `status` = ?, `updated_at` = ? WHERE `id` = ? AND `status` = ? AND `buyer_id` = ?",
		TransactionEvidenceStatusDone,
		time.Now(),
		transactionEvidenceID,
		TransactionEvidenceStatusWaitDone,
		buyer.ID,
	)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusForbidden, "権限がありません")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	_, err = tx.Exec("UPDATE `shippings` SET `status` = ?, `updated_at` = ? WHERE `transaction_evidence_id` = ?",
		ShippingsStatusDone,
		time.Now(),
		transactionEvidenceID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	_, err = tx.Exec("UPDATE `items` SET `status` = ?, `updated_at` = ? WHERE `id` = ?",
		ItemStatusSoldOut,
		time.Now(),
		itemID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	ssr := <-ssrCh
	if ssr == nil {
		outputErrorMsg(w, http.StatusInternalServerError, "failed to request to shipment service")
		tx.Rollback()
		return
	}
	if !(ssr.Status == ShippingsStatusDone) {
		outputErrorMsg(w, http.StatusForbidden, "shipment service側で配送中か配送完了になっていません")
		tx.Rollback()
		return
	}

	tx.Commit()

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(resBuy{TransactionEvidenceID: transactionEvidenceID})
}

func postSell(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	description := r.FormValue("description")
	priceStr := r.FormValue("price")
	categoryIDStr := r.FormValue("category_id")

	f, header, err := r.FormFile("image")
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusBadRequest, "image error")
		return
	}
	defer f.Close()

	user, csrfToken, errCode := getUserAndCSRToken(r)
	if r.FormValue("csrf_token") != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	categoryID, err := strconv.Atoi(categoryIDStr)
	if err != nil || categoryID < 0 {
		outputErrorMsg(w, http.StatusBadRequest, "category id error")
		return
	}

	price, err := strconv.Atoi(priceStr)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "price error")
		return
	}

	if name == "" || description == "" || price == 0 || categoryID == 0 {
		outputErrorMsg(w, http.StatusBadRequest, "all parameters are required")

		return
	}

	if price < ItemMinPrice || price > ItemMaxPrice {
		outputErrorMsg(w, http.StatusBadRequest, ItemPriceErrMsg)

		return
	}

	category, err := getCategoryByID(categoryID)
	if err != nil || category.ParentID == 0 {
		log.Print(categoryID, category)
		outputErrorMsg(w, http.StatusBadRequest, "Incorrect category ID")
		return
	}

	img, err := ioutil.ReadAll(f)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "image error")
		return
	}

	ext := filepath.Ext(header.Filename)

	if !(ext == ".jpg" || ext == ".jpeg" || ext == ".png" || ext == ".gif") {
		outputErrorMsg(w, http.StatusBadRequest, "unsupported image format error")
		return
	}

	if ext == ".jpeg" {
		ext = ".jpg"
	}

	imgName := fmt.Sprintf("%s%s", secureRandomStr(16), ext)
	err = ioutil.WriteFile(fmt.Sprintf("../public/upload/%s", imgName), img, 0644)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "Saving image failed")
		return
	}

	tx := dbx.MustBegin()

	seller := User{}
	err = tx.Get(&seller, "SELECT * FROM `users` WHERE `id` = ? FOR UPDATE", user.ID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "user not found")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	result, err := tx.Exec("INSERT INTO `items` (`seller_id`, `status`, `name`, `price`, `description`,`image_name`,`category_id`) VALUES (?, ?, ?, ?, ?, ?, ?)",
		seller.ID,
		ItemStatusOnSale,
		name,
		price,
		description,
		imgName,
		category.ID,
	)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	itemID, err := result.LastInsertId()
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	now := time.Now()
	_, err = tx.Exec("UPDATE `users` SET `num_sell_items`=?, `last_bump`=? WHERE `id`=?",
		seller.NumSellItems+1,
		now,
		seller.ID,
	)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}
	tx.Commit()

	mapItemID[itemID] = seller.ID

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(resSell{ID: itemID})
}

func secureRandomStr(b int) string {
	k := make([]byte, b)
	if _, err := crand.Read(k); err != nil {
		panic(err)
	}
	return fmt.Sprintf("%x", k)
}

func postBump(w http.ResponseWriter, r *http.Request) {
	rb := reqBump{}
	err := json.NewDecoder(r.Body).Decode(&rb)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	itemID := rb.ItemID

	user, csrfToken, errCode := getUserAndCSRToken(r)
	if rb.CSRFToken != csrfToken {
		outputErrorMsg(w, http.StatusUnprocessableEntity, "csrf token error")
		return
	}
	if errCode != http.StatusOK {
		outputErrorMsg(w, errCode, "session error")
		return
	}

	tx := dbx.MustBegin()

	targetItem := Item{}
	err = tx.Get(&targetItem, "SELECT * FROM `items` WHERE `id` = ? FOR UPDATE", itemID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "item not found")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	if targetItem.SellerID != user.ID {
		outputErrorMsg(w, http.StatusForbidden, "自分の商品以外は編集できません")
		tx.Rollback()
		return
	}

	seller := User{}
	err = tx.Get(&seller, "SELECT * FROM `users` WHERE `id` = ? FOR UPDATE", user.ID)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusNotFound, "user not found")
		tx.Rollback()
		return
	}
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	now := time.Now()
	// last_bump + 3s > now
	if seller.LastBump.Add(BumpChargeSeconds).After(now) {
		outputErrorMsg(w, http.StatusForbidden, "Bump not allowed")
		tx.Rollback()
		return
	}

	_, err = tx.Exec("UPDATE `items` SET `created_at`=?, `updated_at`=? WHERE id=?",
		now,
		now,
		targetItem.ID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	_, err = tx.Exec("UPDATE `users` SET `last_bump`=? WHERE id=?",
		now,
		seller.ID,
	)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	err = tx.Get(&targetItem, "SELECT * FROM `items` WHERE `id` = ?", itemID)
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		tx.Rollback()
		return
	}

	tx.Commit()

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(&resItemEdit{
		ItemID:        targetItem.ID,
		ItemPrice:     targetItem.Price,
		ItemCreatedAt: targetItem.CreatedAt.Unix(),
		ItemUpdatedAt: targetItem.UpdatedAt.Unix(),
	})
}

func getSettings(w http.ResponseWriter, r *http.Request) {
	user, csrfToken, errCode := getUserAndCSRToken(r)

	ress := resSetting{}
	if errCode == http.StatusOK {
		ress.User = &user
	}
	ress.CSRFToken = csrfToken

	ress.PaymentServiceURL = getPaymentServiceURL()
	ress.Categories = categories

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(ress)
}

func postLogin(w http.ResponseWriter, r *http.Request) {
	rl := reqLogin{}
	err := json.NewDecoder(r.Body).Decode(&rl)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	accountName := rl.AccountName
	password := rl.Password

	if accountName == "" || password == "" {
		outputErrorMsg(w, http.StatusBadRequest, "all parameters are required")

		return
	}

	u := User{}
	err = dbx.Get(&u, "SELECT * FROM `users` WHERE `account_name` = ?", accountName)
	if err == sql.ErrNoRows {
		outputErrorMsg(w, http.StatusUnauthorized, "アカウント名かパスワードが間違えています")
		return
	}
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	err = bcrypt.CompareHashAndPassword(u.HashedPassword, []byte(password))
	if err == bcrypt.ErrMismatchedHashAndPassword {
		outputErrorMsg(w, http.StatusUnauthorized, "アカウント名かパスワードが間違えています")
		return
	}
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "crypt error")
		return
	}

	session := getSession(r)

	session.Values["user_id"] = u.ID
	session.Values["csrf_token"] = secureRandomStr(20)
	if err = session.Save(r, w); err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "session error")
		return
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(u)
}

func postRegister(w http.ResponseWriter, r *http.Request) {
	rr := reqRegister{}
	err := json.NewDecoder(r.Body).Decode(&rr)
	if err != nil {
		outputErrorMsg(w, http.StatusBadRequest, "json decode error")
		return
	}

	accountName := rr.AccountName
	address := rr.Address
	password := rr.Password

	if accountName == "" || password == "" || address == "" {
		outputErrorMsg(w, http.StatusBadRequest, "all parameters are required")

		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "error")
		return
	}

	result, err := dbx.Exec("INSERT INTO `users` (`account_name`, `hashed_password`, `address`) VALUES (?, ?, ?)",
		accountName,
		hashedPassword,
		address,
	)
	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	userID, err := result.LastInsertId()

	if err != nil {
		log.Print(err)

		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	u := User{
		ID:          userID,
		AccountName: accountName,
		Address:     address,
	}

	session := getSession(r)
	session.Values["user_id"] = u.ID
	session.Values["csrf_token"] = secureRandomStr(20)
	if err = session.Save(r, w); err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "session error")
		return
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(u)
}

func getReports(w http.ResponseWriter, r *http.Request) {
	transactionEvidences := make([]TransactionEvidence, 0)
	err := dbx.Select(&transactionEvidences, "SELECT * FROM `transaction_evidences` WHERE `id` > 15007")
	if err != nil {
		log.Print(err)
		outputErrorMsg(w, http.StatusInternalServerError, "db error")
		return
	}

	w.Header().Set("Content-Type", "application/json;charset=utf-8")
	json.NewEncoder(w).Encode(transactionEvidences)
}

func outputErrorMsg(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json;charset=utf-8")

	w.WriteHeader(status)

	json.NewEncoder(w).Encode(struct {
		Error string `json:"error"`
	}{Error: msg})
}

func getImageURL(imageName string) string {
	return fmt.Sprintf("/upload/%s", imageName)
}
