package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// 主 MarkdownTable
type MarkdownTable struct {
	Name         string
	Version      string
	Description  string
	LicenseName  string
	PublishTime  string
	GithubStars  string
	OhpmLikes    string
	Points       string
	Popularity   string
	Issues       string
	PullRequests string
	Contributors string
}

// 主 Package 信息
type PackageInfo struct {
	Code                   int // 0: error 1：success
	Name                   string
	Version                string
	Description            string
	Homepage               string
	Repository             string
	PublishTime            int
	Points                 int
	Likes                  int
	Popularity             int
	Downloads              int
	GithubUser             string
	GithubRepo             string
	GithubBaseInfo         GithubBaseInfo
	GithubContributorsInfo []GithubContributorsInfo
}

type GithubBaseInfo struct {
	StargazersCount int `json:"stargazers_count"`
	ForksCount      int `json:"forks_count"`
	OpenIssuesCount int `json:"open_issues_count"`
	License         struct {
		Name string `json:"name"`
	} `json:"license"`
	ContributorsTotal int
}

type GithubContributorsInfo struct {
	Login     string `json:"login"`
	AvatarUrl string `json:"avatar_url"`
	HtmlUrl   string `json:"html_url"`
	Type      string `json:"type"`
}

type PackageBaseInfo struct {
	Body struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Homepage    string `json:"homepage"`
		Repository  string `json:"repository"`
		PublishTime int    `json:"publishTime"`
		Points      int    `json:"points"`
		Likes       int    `json:"likes"`
		Popularity  int    `json:"popularity"`
		Downloads   int    `json:"downloads"`
	} `json:"body"`
}

type PackageDescriptionInfo struct {
	Body struct {
		Rows []struct {
			Description string `json:"description"`
		} `json:"rows"`
	} `json:"body"`
}

type PublisherInfo struct {
	Body struct {
		Rows []struct {
			Name string `json:"name"`
		} `json:"rows"`
	} `json:"body"`
}

func main() {
	var githubToken, filename, publisherList, packageList, sortField, sortMode string
	flag.StringVar(&githubToken, "githubToken", "Github Token with repo permissions", "Github Token with repo permissions")
	flag.StringVar(&filename, "filename", "README.md", "文件名 如: README.md")
	flag.StringVar(&publisherList, "publisherList", "", "publisher ID https://ohpm.openharmony.cn/#/cn/publisher/6542179b6dad4e55f6635764 如: 6542179b6dad4e55f6635764,xxx,xxx")
	flag.StringVar(&packageList, "packageList", "", "package 如: @candies/extended_text,@bb/xx,@cc/xx")
	flag.StringVar(&sortField, "sortField", "name", "name | publishTime | ohpmLikes | githubStars")
	flag.StringVar(&sortMode, "sortMode", "asc", "asc | desc")
	flag.Parse()

	var packageAllList string
	publisherPackageList := getPublisherPackages(publisherList)
	packageAllList = publisherPackageList + "," + packageList
	packageInfoList := getPackageInfo(githubToken, packageAllList)
	sortPackageInfo(packageInfoList, sortField, sortMode)
	markdownTable := assembleMarkdownTable(packageInfoList, sortField)

	// 更新表格
	updateMarkdownTable(filename, markdownTable)
	// 更新总数
	updateMarkdownPackageTotal(filename, len(packageInfoList))
}

// 通过 Publisher 获取所有 Package 名称
// [publisherId] publisher ID 列表(逗号,分割)
// Return 与 packageList 相同的 package 名称列表(逗号,分割)
func getPublisherPackages(publisherId string) string {
	printErrTitle := "🌏⚠️ PublisherPackages: "
	if publisherId == "" {
		return ""
	}
	publisherList := removeDuplicates(strings.Split(publisherId, ","))
	fmt.Println("🌏", publisherList)
	packageNameList := []string{}
	for _, value := range publisherList {
		if value == "" {
			continue
		}
		publisherId := strings.TrimSpace(value)

		// 查找每一页
		pageIndex := 1
		for pageIndex != 0 {
			fmt.Println("🌏🔗 Publisher: " + publisherId + ", Page: " + strconv.Itoa(pageIndex))
			res, err := http.Get("https://ohpm.openharmony.cn/ohpmweb/registry/oh-package/openapi/v1/search?publisherId=" + publisherId + "&pageNum=" + strconv.Itoa(pageIndex) + "&pageSize=10&sortedType=latest&isHomePage=false&condition=")
			if err != nil {
				fmt.Println(printErrTitle, err)
			}
			defer res.Body.Close()
			jsonData, err := io.ReadAll(res.Body)
			if err != nil {
				fmt.Println(printErrTitle, err)
			}
			data := PublisherInfo{}
			if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
				fmt.Println(printErrTitle, err)
			}
			if len(data.Body.Rows) > 0 {
				for _, packageRow := range data.Body.Rows {
					if packageRow.Name != "" {
						packageNameList = append(packageNameList, packageRow.Name)
					}
				}
				pageIndex++
			} else {
				pageIndex = 0
			}
		}
	}
	return strings.Join(packageNameList, ",")
}

// 获取 Package 信息
// [githubToken] Github Token
// [packagesName] package 名称列表(逗号,分割)
func getPackageInfo(githubToken string, packagesName string) []PackageInfo {
	printErrTitle := "📦⚠️ PackageInfo: "
	packageList := removeDuplicates(strings.Split(packagesName, ","))
	fmt.Println("📦", packageList)
	packageInfoList := []PackageInfo{}
	for _, value := range packageList {
		if value == "" {
			continue
		}
		fmt.Println("📦🔥 " + value)
		packageName := strings.TrimSpace(value)
		res, err := http.Get("https://ohpm.openharmony.cn/ohpmweb/registry/oh-package/openapi/v1/detail/" + url.PathEscape(packageName))
		if err != nil {
			fmt.Println(printErrTitle, err)
		}
		defer res.Body.Close()
		jsonData, err := io.ReadAll(res.Body)
		if err != nil {
			fmt.Println(printErrTitle, err)
		}
		var data PackageBaseInfo
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			fmt.Println(printErrTitle, err)
		}

		if data.Body.Name != "" {
			// 可获取信息
			packageInfo := PackageInfo{
				Code:        1,
				Name:        data.Body.Name,
				Version:     data.Body.Version,
				Homepage:    data.Body.Homepage,
				Repository:  data.Body.Repository,
				PublishTime: data.Body.PublishTime,
				Points:      data.Body.Points,
				Likes:       data.Body.Likes,
				Popularity:  data.Body.Popularity,
				Downloads:   data.Body.Downloads,
				Description: getPackageDescriptionInfo(data.Body.Name),
			}
			getGithubInfo(githubToken, &packageInfo)
			packageInfoList = append(packageInfoList, packageInfo)
			fmt.Println("📦✅ " + packageName + ", Code: 1")
		} else {
			// 无法获取信息
			packageInfoList = append(
				packageInfoList,
				PackageInfo{
					Code: 0,
					Name: packageName,
				},
			)
			fmt.Println("📦❌ " + packageName + ", Code: 0")
		}
	}
	return packageInfoList
}

// 获取 Package 描述 信息
// [packageName] 单个 package 名称
func getPackageDescriptionInfo(packageName string) string {
	printErrTitle := "📦⚠️ PackageDescriptionInfo: "
	res, err := http.Get("https://ohpm.openharmony.cn/ohpmweb/registry/oh-package/openapi/v1/search?condition=name:" + url.PathEscape(packageName) + "&pageNum=1&pageSize=10&sortedType=relevancy&isHomePage=false")
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	defer res.Body.Close()
	jsonData, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	var data PackageDescriptionInfo
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		fmt.Println(printErrTitle, err)
	}

	if len(data.Body.Rows) > 0 {
		return data.Body.Rows[0].Description
	}
	return ""
}

// 获取 Github 信息
// [githubToken] Github Token
// [packageInfo] 当前 package 信息
func getGithubInfo(githubToken string, packageInfo *PackageInfo) {
	if packageInfo.Code == 0 {
		return
	}
	finish := false
	var user, repo string
	user, repo = formatGithubInfo(packageInfo.Repository)
	if repo != "" && !finish {
		packageInfo.GithubUser = user
		packageInfo.GithubRepo = repo
		finish = true
	}
	user, repo = formatGithubInfo(packageInfo.Homepage)
	if repo != "" && !finish {
		packageInfo.GithubUser = user
		packageInfo.GithubRepo = repo
		finish = true
	}
	// 获取 Github 相关信息
	if packageInfo.GithubUser != "" && packageInfo.GithubRepo != "" {
		packageInfo.GithubBaseInfo = getGithubBaseInfo(githubToken, packageInfo.GithubUser, packageInfo.GithubRepo)
		packageInfo.GithubContributorsInfo, packageInfo.GithubBaseInfo.ContributorsTotal = getGithubContributorsInfo(githubToken, packageInfo.GithubUser, packageInfo.GithubRepo)
	}
}

// 获取 Github 基础信息
// [githubToken] Github Token
// [user] 用户
// [repo] 仓库
func getGithubBaseInfo(githubToken string, user string, repo string) GithubBaseInfo {
	printErrTitle := "📦⚠️ GithubBaseInfo: "
	client := &http.Client{}
	resp, err := http.NewRequest("GET", "https://api.github.com/repos/"+user+"/"+repo, strings.NewReader(""))
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	resp.Header.Set("Authorization", "bearer "+githubToken)
	resp.Header.Set("Accept", "application/vnd.github+json")
	resp.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := client.Do(resp)
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	defer res.Body.Close()
	jsonData, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	var data GithubBaseInfo
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		fmt.Println(printErrTitle, err)
	}

	return data
}

// 获取 Github 贡献者信息
// [githubToken] Github Token
// [user] 用户
// [repo] 仓库
//
// @return (贡献者列表, 贡献者总数（最多100）)
func getGithubContributorsInfo(githubToken string, user string, repo string) ([]GithubContributorsInfo, int) {
	printErrTitle := "📦⚠️ GithubContributorsInfo: "
	client := &http.Client{}
	resp, err := http.NewRequest("GET", "https://api.github.com/repos/"+user+"/"+repo+"/contributors?page=1&per_page=100", strings.NewReader(""))
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	resp.Header.Set("Authorization", "bearer "+githubToken)
	resp.Header.Set("Accept", "application/vnd.github+json")
	resp.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	res, err := client.Do(resp)
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	defer res.Body.Close()
	jsonData, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println(printErrTitle, err)
	}
	var data []GithubContributorsInfo
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		fmt.Println(printErrTitle, err)
	}

	githubContributorsInfo := []GithubContributorsInfo{}
	i := 1
	/// 取前 3 位非 Bot 贡献者
	for _, value := range data {
		if i > 3 {
			break
		}
		if value.Type == "User" {
			githubContributorsInfo = append(githubContributorsInfo, value)
			i++
		}
	}
	return githubContributorsInfo, len(data)
}

// 格式化 Github 信息
// Return (githubUser, githubRepo)
func formatGithubInfo(value string) (string, string) {
	var githubUser, githubRepo string
	result := regexp.MustCompile(`(?:github.com/).*`).FindAllString(value, -1)
	if result != nil {
		info := strings.Split(result[0], "/")
		if len(info) >= 3 {
			githubUser = info[1]
			githubRepo = strings.ReplaceAll(info[2], ".git", "")
		}
	}
	return githubUser, githubRepo
}

// 排序
// [packageInfoList] 	信息列表
// [sortField] 				排序字段 可选：name(default) | publishTime | ohpmLikes | githubStars
// [sortMode] 				排序方式 可选：asc(default) | desc
func sortPackageInfo(packageInfoList []PackageInfo, sortField string, sortMode string) {
	switch sortField {
	case "name":
		// 按照 名称 排序
		sort.SliceStable(packageInfoList, func(i, j int) bool {
			iData := packageInfoList[i].Name
			jData := packageInfoList[j].Name
			switch sortMode {
			case "asc":
				return iData < jData
			case "desc":
				return iData > jData
			default:
				return iData < jData
			}
		})
	case "publishTime":
		// 按 最新发布时间 排序
		sort.SliceStable(packageInfoList, func(i, j int) bool {
			iData := packageInfoList[i].PublishTime
			jData := packageInfoList[j].PublishTime
			switch sortMode {
			case "asc":
				return iData > jData
			case "desc":
				return iData < jData
			default:
				return iData > jData
			}
		})
	case "ohpmLikes":
		// 按 ohpm likes 排序
		sort.SliceStable(packageInfoList, func(i, j int) bool {
			iData := packageInfoList[i].Likes
			jData := packageInfoList[j].Likes
			switch sortMode {
			case "asc":
				return iData < jData
			case "desc":
				return iData > jData
			default:
				return iData < jData
			}
		})
	case "githubStars":
		// 按 github stars 排序
		sort.SliceStable(packageInfoList, func(i, j int) bool {
			iData := packageInfoList[i].GithubBaseInfo.StargazersCount
			jData := packageInfoList[j].GithubBaseInfo.StargazersCount
			switch sortMode {
			case "asc":
				return iData < jData
			case "desc":
				return iData > jData
			default:
				return iData < jData
			}
		})
	default:
		// 按照 名称 排序
		sort.SliceStable(packageInfoList, func(i, j int) bool {
			iData := packageInfoList[i].Name
			jData := packageInfoList[j].Name
			switch sortMode {
			case "asc":
				return iData < jData
			case "desc":
				return iData > jData
			default:
				return iData < jData
			}
		})
	}
}

// 组装表格内容
// [packageInfoList] 	信息列表
// [sortField] 				排序字段 可选：name(default) | publishTime | ohpmLikes | githubStars
// [sortMode] 				排序方式 可选：asc(default) | desc
func assembleMarkdownTable(packageInfoList []PackageInfo, sortField string) string {
	markdownTableList := []MarkdownTable{}
	for _, value := range packageInfoList {
		var name, version, licenseName, publishTime, githubStars, ohpmLikes, points, popularity, issues, pullRequests, contributors string
		switch value.Code {
		case 0:
			// 无法获取信息
			name = value.Name + " ⁉️"
		case 1:
			// 已获取信息
			// Base
			const ohpmLogo = "data:image/x-icon;base64,iVBORw0KGgoAAAANSUhEUgAAAMgAAADICAYAAACtWK6eAAAAGXRFWHRTb2Z0d2FyZQBBZG9iZSBJbWFnZVJlYWR5ccllPAAAA3RpVFh0WE1MOmNvbS5hZG9iZS54bXAAAAAAADw/eHBhY2tldCBiZWdpbj0i77u/IiBpZD0iVzVNME1wQ2VoaUh6cmVTek5UY3prYzlkIj8+IDx4OnhtcG1ldGEgeG1sbnM6eD0iYWRvYmU6bnM6bWV0YS8iIHg6eG1wdGs9IkFkb2JlIFhNUCBDb3JlIDcuMS1jMDAwIDc5Ljk4ZDc5NDIsIDIwMjIvMDMvMjEtMTE6NDA6NTkgICAgICAgICI+IDxyZGY6UkRGIHhtbG5zOnJkZj0iaHR0cDovL3d3dy53My5vcmcvMTk5OS8wMi8yMi1yZGYtc3ludGF4LW5zIyI+IDxyZGY6RGVzY3JpcHRpb24gcmRmOmFib3V0PSIiIHhtbG5zOnhtcE1NPSJodHRwOi8vbnMuYWRvYmUuY29tL3hhcC8xLjAvbW0vIiB4bWxuczpzdFJlZj0iaHR0cDovL25zLmFkb2JlLmNvbS94YXAvMS4wL3NUeXBlL1Jlc291cmNlUmVmIyIgeG1sbnM6eG1wPSJodHRwOi8vbnMuYWRvYmUuY29tL3hhcC8xLjAvIiB4bXBNTTpPcmlnaW5hbERvY3VtZW50SUQ9InhtcC5kaWQ6YjViZmI3ZDQtMzViMS1jYTQwLWFjNTQtNTRiNzBiNTJjOGZlIiB4bXBNTTpEb2N1bWVudElEPSJ4bXAuZGlkOkUxRTg1QzYzNjBDNDExRURBRTlGRjA5RjhGQTg1QTA1IiB4bXBNTTpJbnN0YW5jZUlEPSJ4bXAuaWlkOkUxRTg1QzYyNjBDNDExRURBRTlGRjA5RjhGQTg1QTA1IiB4bXA6Q3JlYXRvclRvb2w9IkFkb2JlIFBob3Rvc2hvcCAyMi41IChXaW5kb3dzKSI+IDx4bXBNTTpEZXJpdmVkRnJvbSBzdFJlZjppbnN0YW5jZUlEPSJ4bXAuaWlkOmI1YmZiN2Q0LTM1YjEtY2E0MC1hYzU0LTU0YjcwYjUyYzhmZSIgc3RSZWY6ZG9jdW1lbnRJRD0ieG1wLmRpZDpiNWJmYjdkNC0zNWIxLWNhNDAtYWM1NC01NGI3MGI1MmM4ZmUiLz4g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"
			name = "[" + value.Name + "](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			version = "v" + value.Version
			publishTime = "<strong>PublishTime:</strong> " + strconv.Itoa(value.PublishTime)
			githubStars = ""
			ohpmLikes = "[![OHPM likes](https://img.shields.io/badge/" + strconv.Itoa(value.Likes) + "-_?style=social&logo=" + ohpmLogo + "&logoColor=168AFD&label=)](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			points = "[![OHPM points](https://img.shields.io/badge/" + strconv.Itoa(value.Points) + "-_?style=flat&label=Points&labelColor=4CC71E&color=5EDE2E)](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			popularity = "[![OHPM popularity](https://img.shields.io/badge/" + strconv.Itoa(value.Popularity) + "-_?style=flat&label=Popularity&labelColor=4CC71E&color=5EDE2E)](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			issues = "-"
			pullRequests = "-"

			// Github
			if value.GithubUser != "" && value.GithubRepo != "" {
				githubURL := value.GithubUser + "/" + value.GithubRepo
				licenseName = "<strong>License:</strong> "
				if value.GithubBaseInfo.License.Name != "" {
					licenseName += value.GithubBaseInfo.License.Name
				} else {
					licenseName += "-"
				}
				githubStars = "[![GitHub stars](https://img.shields.io/github/stars/" + githubURL + "?style=social&logo=github&logoColor=1F2328&label=)](https://github.com/" + githubURL + ")"
				issues = "[![GitHub issues](https://img.shields.io/github/issues/" + githubURL + "?label=)](https://github.com/" + githubURL + "/issues)"
				pullRequests = "[![GitHub pull requests](https://img.shields.io/github/issues-pr/" + githubURL + "?label=)](https://github.com/" + githubURL + "/pulls)"

				// contributors begin
				if len(value.GithubContributorsInfo) > 0 {
					var githubContributorsInfoList = value.GithubContributorsInfo
					contributors += `<table align="center" border="0">`

					// contributors
					switch len(value.GithubContributorsInfo) {
					case 1:
						contributors += `<tr align="center">`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[0].HtmlUrl + `"><img width="36px" src="` + githubContributorsInfoList[0].AvatarUrl + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
					case 2:
						contributors += `<tr align="center">`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[0].HtmlUrl + `"><img width="30px" src="` + githubContributorsInfoList[0].AvatarUrl + `" /></a>`
						contributors += `</td>`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[1].HtmlUrl + `"><img width="30px" src="` + githubContributorsInfoList[1].AvatarUrl + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
					case 3:
						contributors += `<tr align="center">`
						contributors += `<td colspan="2">`
						contributors += `<a href="` + githubContributorsInfoList[0].HtmlUrl + `"><img width="36px" src="` + githubContributorsInfoList[0].AvatarUrl + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
						contributors += `<tr align="center">`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[1].HtmlUrl + `"><img width="30px" src="` + githubContributorsInfoList[1].AvatarUrl + `" /></a>`
						contributors += `</td>`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[2].HtmlUrl + `"><img width="30px" src="` + githubContributorsInfoList[2].AvatarUrl + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
					}

					// total
					contributors += `<tr align="center">`
					contributors += `<td colspan="2">`
					if value.GithubBaseInfo.ContributorsTotal >= 100 {
						contributors += `<a href="https://github.com/` + githubURL + `/graphs/contributors">Total: 99+</a>`
					} else {
						contributors += `<a href="https://github.com/` + githubURL + `/graphs/contributors">Total: ` + strconv.Itoa(value.GithubBaseInfo.ContributorsTotal) + `</a>`
					}
					contributors += `</td>`
					contributors += `</tr>`

					contributors += `</table>`
				}
				// contributors end
			}
		}
		markdownTableList = append(
			markdownTableList,
			MarkdownTable{
				Name:         name,
				Version:      version,
				Description:  value.Description,
				LicenseName:  licenseName,
				PublishTime:  publishTime,
				GithubStars:  githubStars,
				OhpmLikes:    ohpmLikes,
				Points:       points,
				Popularity:   popularity,
				Issues:       issues,
				PullRequests: pullRequests,
				Contributors: contributors,
			},
		)
	}

	markdown := ""
	markdown += "<sub>Sort by " + sortField + " | Total " + strconv.Itoa(len(markdownTableList)) + "</sub> \n\n" +
		"| <sub>Package</sub> | <sub>Stars/Likes</sub> | <sub>Points / Popularity</sub> | <sub>Issues / Pull_requests</sub> | <sub>Contributors</sub> | \n" +
		"|--------------------|------------------------|------------------------------|-----------------------------------|:-----------------------:| \n"
	for _, value := range markdownTableList {
		markdown += "" +
			"| " + value.Name + " <sup><strong>" + value.Version + "</strong></sup> <br/> <sub>" + formatString(value.Description) + "</sub> <br/> <sub>" + value.LicenseName + "</sub> <br/> <sub>" + value.PublishTime + "</sub>" +
			" | " + value.GithubStars + " <br/> " + value.OhpmLikes +
			" | " + value.Points + " <br/> " + value.Popularity +
			" | " + value.Issues + " <br/> " + value.PullRequests +
			" | " + value.Contributors +
			" | \n"
	}
	return markdown
}

// 更新 Markdown 表格
// [filename]	更新的文件
// [markdown]	更新内容
//
// <!-- md:OHPMDashboard begin --><!-- md:OHPMDashboard end -->
func updateMarkdownTable(filename string, markdown string) error {
	md, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("📄❌ updateMarkdownTable: Error reade a file: %w", err)
	}

	begin := "<!-- md:OHPMDashboard begin -->"
	end := "<!-- md:OHPMDashboard end -->"
	newMdText := bytes.NewBuffer(nil)
	newMdText.WriteString(begin)
	newMdText.WriteString(" \n")
	newMdText.WriteString(markdown)
	newMdText.WriteString(" \n")
	newMdText.WriteString("Updated on " + time.Now().Format(time.RFC3339) + " by [Action](https://github.com/AmosHuKe/ohpm-dashboard). \n")
	newMdText.WriteString(end)

	reg := regexp.MustCompile(begin + "(?s)(.*?)" + end)
	newMd := reg.ReplaceAll(md, newMdText.Bytes())

	err = os.WriteFile(filename, newMd, os.ModeAppend)
	if err != nil {
		return fmt.Errorf("📄❌ updateMarkdownTable: Error writing a file: %w", err)
	}
	fmt.Println("📄✅ updateMarkdownTable: Success")
	return nil
}

// 更新 Markdown Package 总数计数
// [filename]	更新的文件
// [total]		总数
//
// <!-- md:OHPMDashboard-total begin --><!-- md:OHPMDashboard-total end -->
func updateMarkdownPackageTotal(filename string, total int) error {
	md, err := os.ReadFile(filename)
	if err != nil {
		return fmt.Errorf("📄❌ updateMarkdownPackageTotal: Error reade a file: %w", err)
	}

	begin := "<!-- md:OHPMDashboard-total begin -->"
	end := "<!-- md:OHPMDashboard-total end -->"
	newMdText := bytes.NewBuffer(nil)
	newMdText.WriteString(begin)
	newMdText.WriteString(strconv.Itoa(total))
	newMdText.WriteString(end)

	reg := regexp.MustCompile(begin + "(?s)(.*?)" + end)
	newMd := reg.ReplaceAll(md, newMdText.Bytes())

	err = os.WriteFile(filename, newMd, os.ModeAppend)
	if err != nil {
		return fmt.Errorf("📄❌ updateMarkdownPackageTotal: Error writing a file: %w", err)
	}
	fmt.Println("📄✅ updateMarkdownPackageTotal: Success")
	return nil
}

// 格式化字符串
func formatString(v string) string {
	value := v
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "丨")
	return value
}

// 去重
func removeDuplicates(arr []string) []string {
	uniqueMap := make(map[string]bool)
	for _, v := range arr {
		if _, ok := uniqueMap[v]; !ok {
			uniqueMap[v] = true
		}
	}
	var uniqueArr []string
	for k := range uniqueMap {
		uniqueArr = append(uniqueArr, k)
	}
	return uniqueArr
}
