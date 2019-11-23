package main

import "sort"

type Category struct {
	ID                 int    `json:"id" db:"id"`
	ParentID           int    `json:"parent_id" db:"parent_id"`
	CategoryName       string `json:"category_name" db:"category_name"`
	ParentCategoryName string `json:"parent_category_name,omitempty" db:"-"`
}

var embedCategories = []*Category{
	{1, 0, "ソファー", ""},
	{2, 1, "一人掛けソファー", ""},
	{3, 1, "二人掛けソファー", ""},
	{4, 1, "コーナーソファー", ""},
	{5, 1, "二段ソファー", ""},
	{6, 1, "ソファーベッド", ""},
	{10, 0, "家庭用チェア", ""},
	{11, 10, "スツール", ""},
	{12, 10, "クッションスツール", ""},
	{13, 10, "ダイニングチェア", ""},
	{14, 10, "リビングチェア", ""},
	{15, 10, "カウンターチェア", ""},
	{20, 0, "キッズチェア", ""},
	{21, 20, "学習チェア", ""},
	{22, 20, "ベビーソファ", ""},
	{23, 20, "キッズハイチェア", ""},
	{24, 20, "テーブルチェア", ""},
	{30, 0, "オフィスチェア", ""},
	{31, 30, "デスクチェア", ""},
	{32, 30, "ビジネスチェア", ""},
	{33, 30, "回転チェア", ""},
	{34, 30, "リクライニングチェア", ""},
	{35, 30, "投擲用椅子", ""},
	{40, 0, "折りたたみ椅子", ""},
	{41, 40, "パイプ椅子", ""},
	{42, 40, "木製折りたたみ椅子", ""},
	{43, 40, "キッチンチェア", ""},
	{44, 40, "アウトドアチェア", ""},
	{45, 40, "作業椅子", ""},
	{50, 0, "ベンチ", ""},
	{51, 50, "一人掛けベンチ", ""},
	{52, 50, "二人掛けベンチ", ""},
	{53, 50, "アウトドア用ベンチ", ""},
	{54, 50, "収納付きベンチ", ""},
	{55, 50, "背もたれ付きベンチ", ""},
	{56, 50, "ベンチマーク", ""},
	{60, 0, "座椅子", ""},
	{61, 60, "和風座椅子", ""},
	{62, 60, "高座椅子", ""},
	{63, 60, "ゲーミング座椅子", ""},
	{64, 60, "ロッキングチェア", ""},
	{65, 60, "座布団", ""},
	{66, 60, "空気椅子", ""},
}

func initCategories() (map[int]*Category, []Category) {
	categoryMap := make(map[int]*Category)
	for _, v := range embedCategories {
		categoryMap[v.ID] = v
	}
	for _, v := range categoryMap {
		if v.ParentID != 0 {
			v.ParentCategoryName = categoryMap[v.ParentID].CategoryName
		}
	}

	categories := make([]Category, 0, len(categoryMap))
	for _, v := range categoryMap {
		categories = append(categories, *v)
	}
	sort.Slice(categories, func(i, j int) bool {
		return categories[i].ID < categories[j].ID
	})

	return categoryMap, categories
}
