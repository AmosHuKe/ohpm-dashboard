// ohpm.openharmony.cn Package 仪表盘
//
// 识别并更新指定 [filename] Markdown 文件中的特定占位内容，
//
// 特定占位:
//   - `<!-- md:OHPMDashboard begin --><!-- md:OHPMDashboard end -->`              仪表盘表格（Markdown 格式）
//   - `<!-- md:OHPMDashboard-total begin --><!-- md:OHPMDashboard-total end -->`  Package 数量
//
// 使用:
//   - `go run main.go -githubToken xxx -filename xxx -publisherList xxx -packageList xxx -sortField xxx -sortMode xxx`
//
// 参数:
//   - [githubToken]    拥有 repo 权限的 Github 令牌
//   - [filename]       需要更新的 Markdown 文件，例如："README.md" "test/test.md"
//   - [publisherList]  Publisher ID 列表 (`,`逗号分割) https://ohpm.openharmony.cn/#/cn/publisher/6542179b6dad4e55f6635764 例如："6542179b6dad4e55f6635764,xxx,xxx"
//   - [packageList]    Package 名称列表 (`,`逗号分割)，例如："@candies/extended_text,@bb/xx,@cc/xx"
//   - [sortField]      排序字段 可选：name(default) | publishTime | ohpmLikes | ohpmDownloads | githubStars
//   - [sortMode]       排序方式 可选：asc(default) | desc
package main

import (
	"bytes"
	"context"
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
	"sync"
	"time"
)

const (
	// maxConcurrency 是 package 级别的并发抓取上限。
	// 需要按 ohpm.openharmony.cn 和 GitHub 限流取保守值。
	maxConcurrency = 8
	// maxAttempts 是单个 HTTP 请求的最大尝试次数（含首次）。
	maxAttempts = 3
	// httpTimeout 是单个 HTTP 请求的超时时间，防止请求挂起拖跨整个 Action。
	httpTimeout = 30 * time.Second
	// retryBaseDelay 是重试的基础退避时长（指数增长）。
	retryBaseDelay = 500 * time.Millisecond
)

// 主 MarkdownTable 用于存储每个 package 在 Markdown 表格中的展示信息
type MarkdownTable struct {
	Name          string
	Version       string
	Description   string
	LicenseName   string
	PublishTime   string
	GithubStars   string
	OhpmLikes     string
	OhpmDownloads string
	Points        string
	Popularity    string
	Issues        string
	PullRequests  string
	Contributors  string
}

// 主 Package 信息，聚合 package 所有相关的数据
type PackageInfo struct {
	Code                   int // 0: error 1：success
	Name                   string
	Version                string
	LicenseName            string
	Description            string
	Homepage               string
	Repository             string
	PublishTime            int
	Points                 int
	MaxPoints              int
	Likes                  int
	Popularity             int
	Downloads              int
	GithubUser             string
	GithubRepo             string
	GithubBaseInfo         GithubBaseInfo
	GithubContributorsInfo []GithubContributorsInfo
}

// 每个 package 对应 Github 仓库的基础信息
type GithubBaseInfo struct {
	StargazersCount int `json:"stargazers_count"`
	ForksCount      int `json:"forks_count"`
	OpenIssuesCount int `json:"open_issues_count"`
	License         struct {
		Name string `json:"name"`
	} `json:"license"`
	ContributorsTotal int
}

// 每个 package 对应 Github 仓库的贡献者基础信息
type GithubContributorsInfo struct {
	Login     string `json:"login"`
	Id        int    `json:"id"`
	AvatarUrl string `json:"avatar_url"`
	HtmlUrl   string `json:"html_url"`
	Type      string `json:"type"`
}

// ohpm.openharmony.cn package 基础信息
type PackageBaseInfo struct {
	Body struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		License     string `json:"license"`
		Homepage    string `json:"homepage"`
		Repository  string `json:"repository"`
		PublishTime int    `json:"publishTime"`
		Points      int    `json:"points"`
		Likes       int    `json:"likes"`
		Popularity  int    `json:"popularity"`
		Downloads   int    `json:"downloads"`
		PointDetail struct {
			Point int `json:"point"`
		} `json:"pointDetail"`
	} `json:"body"`
}

// ohpm.openharmony.cn package 描述信息
type PackageDescriptionInfo struct {
	Body struct {
		Rows []struct {
			Description string `json:"description"`
		} `json:"rows"`
	} `json:"body"`
}

// ohpm.openharmony.cn publisher 下所有 package 信息
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
	flag.StringVar(&sortField, "sortField", "name", "name | publishTime | ohpmLikes | ohpmDownloads | githubStars")
	flag.StringVar(&sortMode, "sortMode", "asc", "asc | desc")
	flag.Parse()

	ctx := context.Background()
	client := newHTTPClient()

	packageNames, err := mergePackageList(ctx, client, publisherList, packageList)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	packageInfoList, err := getPackageInfo(ctx, client, githubToken, packageNames)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	sortPackageInfo(packageInfoList, sortField, sortMode)
	markdownTable := assembleMarkdownTable(packageInfoList, sortField)

	// 更新表格
	if err := updateMarkdownTable(filename, markdownTable); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	// 更新总数
	if err := updateMarkdownPackageTotal(filename, len(packageInfoList)); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}

// 合并 publisher 的 package 和自定义 package 列表，并去重（保持顺序）
//
// 参数:
//   - [ctx]           上下文
//   - [client]        共享 HTTP Client
//   - [publisherList] publisher ID 列表（逗号,分割）
//   - [packageList]   package 名称列表（逗号,分割）
//
// 返回值:
//   - 合并去重后的 package 名称列表
func mergePackageList(ctx context.Context, client *http.Client, publisherList string, packageList string) ([]string, error) {
	publisherPackages, err := getPublisherPackages(ctx, client, publisherList)
	if err != nil {
		return nil, err
	}
	all := append(publisherPackages, strings.Split(packageList, ",")...)
	return removeDuplicates(all), nil
}

// 通过 Publisher 获取所有 Package 名称
//
// 参数:
//   - [ctx]         上下文
//   - [client]      共享 HTTP Client
//   - [publisherId] publisher ID 列表（逗号,分割）
//
// 返回值:
//   - package 名称列表
func getPublisherPackages(ctx context.Context, client *http.Client, publisherId string) ([]string, error) {
	printErrTitle := "🌏⚠️ PublisherPackages: "
	if strings.TrimSpace(publisherId) == "" {
		return nil, nil
	}
	publisherList := removeDuplicates(strings.Split(publisherId, ","))
	fmt.Println("🌏", publisherList)
	packageNameList := []string{}
	for _, publisher := range publisherList {
		// 逐页查询，直至返回空结果
		for pageIndex := 1; ; pageIndex++ {
			fmt.Printf("🌏🔗 Publisher: %s, Page: %d \n", publisher, pageIndex)
			rawURL := fmt.Sprintf("https://ohpm.openharmony.cn/ohpmweb/registry/oh-package/openapi/v1/search?publisherId=%s&pageNum=%d&pageSize=10&sortedType=latest&isHomePage=false&condition=", url.QueryEscape(publisher), pageIndex)
			body, status, err := httpGetWithRetry(ctx, client, rawURL, nil)
			if err != nil {
				return nil, fmt.Errorf("%s%w", printErrTitle, err)
			}
			if status == http.StatusNotFound {
				break // 无更多结果
			}
			if status != http.StatusOK {
				return nil, fmt.Errorf("%s%s: unexpected status %d", printErrTitle, publisher, status)
			}
			var data PublisherInfo
			if err := json.Unmarshal(body, &data); err != nil {
				return nil, fmt.Errorf("%s%w", printErrTitle, err)
			}
			if len(data.Body.Rows) == 0 {
				break
			}
			for _, row := range data.Body.Rows {
				if row.Name != "" {
					packageNameList = append(packageNameList, row.Name)
				}
			}
		}
	}
	return removeDuplicates(packageNameList), nil
}

// 获取所有 Package 信息（并发抓取）
//
// 以 [maxConcurrency] 为上限并发处理每个 package，结果按输入顺序返回，保证排序前顺序确定。
// 任一 package 抓取失败将取消其余请求并整体返回错误。
//
// 参数:
//   - [ctx]          上下文
//   - [client]       共享 HTTP Client
//   - [githubToken]  Github Token
//   - [packageNames] package 名称列表（已去重清洗）
//
// 返回值:
//   - [PackageInfo] 列表（与 packageNames 顺序一致）
func getPackageInfo(ctx context.Context, client *http.Client, githubToken string, packageNames []string) ([]PackageInfo, error) {
	fmt.Println("📦", packageNames)
	return concurrentMap(ctx, packageNames, maxConcurrency, func(ctx context.Context, name string) (PackageInfo, error) {
		fmt.Println("📦🔥 " + name)
		info, err := fetchPackage(ctx, client, githubToken, name)
		if err != nil {
			return PackageInfo{}, err
		}
		if info.Code == 1 {
			fmt.Printf("📦✅ %s, Code: 1\n", name)
		} else {
			fmt.Printf("📦❌ %s, Code: 0\n", name)
		}
		return info, nil
	})
}

// 抓取单个 package 的全部信息（ohpm 基础信息 -> 描述 -> Github 信息）
//
// 参数:
//   - [ctx]         上下文
//   - [client]      共享 HTTP Client
//   - [githubToken] Github Token
//   - [name]        package 名称
//
// 返回值:
//   - [PackageInfo]，包不存在时 Code=0（降级展示为 ⁉️，非错误）
func fetchPackage(ctx context.Context, client *http.Client, githubToken string, name string) (PackageInfo, error) {
	printErrTitle := "📦⚠️ PackageInfo: "
	rawURL := fmt.Sprintf("https://ohpm.openharmony.cn/ohpmweb/registry/oh-package/openapi/v1/detail/%s", url.PathEscape(name))
	body, status, err := httpGetWithRetry(ctx, client, rawURL, nil)
	if err != nil {
		return PackageInfo{}, fmt.Errorf("%s%w", printErrTitle, err)
	}
	// 404：包不存在 -> 降级
	if status == http.StatusNotFound {
		return PackageInfo{Code: 0, Name: name}, nil
	}
	if status != http.StatusOK {
		return PackageInfo{}, fmt.Errorf("%s%s: unexpected status %d", printErrTitle, name, status)
	}
	var data PackageBaseInfo
	if err := json.Unmarshal(body, &data); err != nil {
		return PackageInfo{}, fmt.Errorf("%s%w", printErrTitle, err)
	}
	if data.Body.Name == "" {
		return PackageInfo{Code: 0, Name: name}, nil
	}

	packageInfo := PackageInfo{
		Code:        1,
		Name:        data.Body.Name,
		Version:     data.Body.Version,
		LicenseName: data.Body.License,
		Homepage:    data.Body.Homepage,
		Repository:  data.Body.Repository,
		PublishTime: data.Body.PublishTime,
		Points:      data.Body.Points,
		MaxPoints:   data.Body.PointDetail.Point,
		Likes:       data.Body.Likes,
		Popularity:  data.Body.Popularity,
		Downloads:   data.Body.Downloads,
	}

	description, err := getPackageDescriptionInfo(ctx, client, data.Body.Name)
	if err != nil {
		return PackageInfo{}, err
	}
	packageInfo.Description = description

	if err := getGithubInfo(ctx, client, githubToken, &packageInfo); err != nil {
		return PackageInfo{}, err
	}
	return packageInfo, nil
}

// 获取 Package 描述信息
//
// 参数:
//   - [ctx]         上下文
//   - [client]      共享 HTTP Client
//   - [packageName] 单个 package 名称
//
// 返回值:
//   - Package 描述信息（404 时降级为空）
func getPackageDescriptionInfo(ctx context.Context, client *http.Client, packageName string) (string, error) {
	printErrTitle := "📦⚠️ PackageDescriptionInfo: "
	rawURL := fmt.Sprintf("https://ohpm.openharmony.cn/ohpmweb/registry/oh-package/openapi/v1/search?condition=name:%s&pageNum=1&pageSize=10&sortedType=relevancy&isHomePage=false", url.PathEscape(packageName))
	body, status, err := httpGetWithRetry(ctx, client, rawURL, nil)
	if err != nil {
		return "", fmt.Errorf("%s%w", printErrTitle, err)
	}
	if status == http.StatusNotFound {
		return "", nil // 无描述数据 -> 降级
	}
	if status != http.StatusOK {
		return "", fmt.Errorf("%s%s: unexpected status %d", printErrTitle, packageName, status)
	}
	var data PackageDescriptionInfo
	if err := json.Unmarshal(body, &data); err != nil {
		return "", fmt.Errorf("%s%w", printErrTitle, err)
	}
	if len(data.Body.Rows) > 0 {
		return data.Body.Rows[0].Description, nil
	}
	return "", nil
}

// 获取 Github 信息，
// 处理 [PackageInfo] 中 GithubUser, GithubRepo, GithubBaseInfo, GithubContributorsInfo 的值
//
// 参数:
//   - [ctx]         上下文
//   - [client]      共享 HTTP Client
//   - [githubToken] Github Token
//   - [packageInfo] 当前 package 信息
func getGithubInfo(ctx context.Context, client *http.Client, githubToken string, packageInfo *PackageInfo) error {
	if packageInfo.Code == 0 {
		return nil
	}
	// 依次尝试 Repository、Homepage 解析 Github 地址，取首个命中
	for _, link := range []string{packageInfo.Repository, packageInfo.Homepage} {
		if user, repo := formatGithubInfo(link); repo != "" {
			packageInfo.GithubUser = user
			packageInfo.GithubRepo = repo
			break
		}
	}
	if packageInfo.GithubUser == "" || packageInfo.GithubRepo == "" {
		return nil
	}

	githubBaseInfo, err := getGithubBaseInfo(ctx, client, githubToken, packageInfo.GithubUser, packageInfo.GithubRepo)
	if err != nil {
		return err
	}
	packageInfo.GithubBaseInfo = githubBaseInfo

	githubContributorsInfo, contributorsTotal, err := getGithubContributorsInfo(ctx, client, githubToken, packageInfo.GithubUser, packageInfo.GithubRepo)
	if err != nil {
		return err
	}
	packageInfo.GithubContributorsInfo = githubContributorsInfo
	packageInfo.GithubBaseInfo.ContributorsTotal = contributorsTotal
	return nil
}

// 构造 GitHub API 通用请求头
func githubHeaders(githubToken string) map[string]string {
	return map[string]string{
		"Authorization":        "bearer " + githubToken,
		"Accept":               "application/vnd.github+json",
		"X-GitHub-Api-Version": "2026-03-10",
	}
}

// 获取 Github 基础信息
//
// 参数:
//   - [ctx]         上下文
//   - [client]      共享 HTTP Client
//   - [githubToken] Github Token
//   - [user]        用户
//   - [repo]        仓库
//
// 返回值:
//   - [GithubBaseInfo] 信息（404 时降级为空）
func getGithubBaseInfo(ctx context.Context, client *http.Client, githubToken string, user string, repo string) (GithubBaseInfo, error) {
	printErrTitle := "📦⚠️ GithubBaseInfo: "
	rawURL := fmt.Sprintf("https://api.github.com/repos/%s/%s", user, repo)
	body, status, err := httpGetWithRetry(ctx, client, rawURL, githubHeaders(githubToken))
	if err != nil {
		return GithubBaseInfo{}, fmt.Errorf("%s%w", printErrTitle, err)
	}
	if status == http.StatusNotFound {
		return GithubBaseInfo{}, nil // 仓库不存在 -> 降级
	}
	if status != http.StatusOK {
		return GithubBaseInfo{}, fmt.Errorf("%s%s/%s: unexpected status %d", printErrTitle, user, repo, status)
	}
	var data GithubBaseInfo
	if err := json.Unmarshal(body, &data); err != nil {
		return GithubBaseInfo{}, fmt.Errorf("%s%w", printErrTitle, err)
	}
	return data, nil
}

// 获取 Github 贡献者信息
//
// 参数:
//   - [ctx]         上下文
//   - [client]      共享 HTTP Client
//   - [githubToken] Github Token
//   - [user]        用户
//   - [repo]        仓库
//
// 返回值:
//   - [GithubContributorsInfo] 贡献者列表（前 3 位非 Bot）
//   - 贡献者总数（最多 100；404/204 时为 0）
func getGithubContributorsInfo(ctx context.Context, client *http.Client, githubToken string, user string, repo string) ([]GithubContributorsInfo, int, error) {
	printErrTitle := "📦⚠️ GithubContributorsInfo: "
	rawURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/contributors?page=1&per_page=100", user, repo)
	body, status, err := httpGetWithRetry(ctx, client, rawURL, githubHeaders(githubToken))
	if err != nil {
		return nil, 0, fmt.Errorf("%s%w", printErrTitle, err)
	}
	// 404（仓库不存在）/ 204（空仓库，无贡献者）-> 降级
	if status == http.StatusNotFound || status == http.StatusNoContent {
		return nil, 0, nil
	}
	if status != http.StatusOK {
		return nil, 0, fmt.Errorf("%s%s/%s: unexpected status %d", printErrTitle, user, repo, status)
	}
	var data []GithubContributorsInfo
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, 0, fmt.Errorf("%s%w", printErrTitle, err)
	}

	githubContributorsInfo := []GithubContributorsInfo{}
	i := 1
	// 取前 3 位非 Bot 贡献者
	for _, value := range data {
		if i > 3 {
			break
		}
		if value.Type == "User" {
			githubContributorsInfo = append(githubContributorsInfo, value)
			i++
		}
	}
	return githubContributorsInfo, len(data), nil
}

// 匹配 github.com/ 之后的 user/repo 路径。
// `.` 已转义，避免误匹配 githubXcom 等相似域名。
var githubURLRegexp = regexp.MustCompile(`github\.com/(.+)`)

// 格式化 Github 信息
//
// 参数:
//   - [value] Github 链接
//
// 返回值:
//   - githubUser 信息
//   - githubRepo 信息
func formatGithubInfo(value string) (string, string) {
	var githubUser, githubRepo string
	result := githubURLRegexp.FindStringSubmatch(value)
	if len(result) >= 2 {
		info := strings.Split(result[1], "/")
		if len(info) >= 2 && info[0] != "" && info[1] != "" {
			githubUser = info[0]
			repo := info[1]
			// 去除 query/fragment 尾巴（如 ?tab=、#readme）及 .git 后缀
			if i := strings.IndexAny(repo, "#?"); i >= 0 {
				repo = repo[:i]
			}
			githubRepo = strings.TrimSuffix(repo, ".git")
		}
	}
	return githubUser, githubRepo
}

// 对 [packageInfoList] 排序
//
// 参数:
//   - [packageInfoList]  信息列表
//   - [sortField]        排序字段 可选：name(default) | publishTime | ohpmLikes | ohpmDownloads | githubStars
//   - [sortMode]         排序方式 可选：asc(default) | desc
func sortPackageInfo(packageInfoList []PackageInfo, sortField string, sortMode string) {
	isDesc := sortMode == "desc"
	sort.SliceStable(packageInfoList, func(i, j int) bool {
		p1 := packageInfoList[i]
		p2 := packageInfoList[j]
		var result bool
		switch sortField {
		case "name":
			// 按照 名称 排序
			result = p1.Name < p2.Name
		case "publishTime":
			// 按 最新发布时间 排序
			result = p1.PublishTime > p2.PublishTime
		case "ohpmLikes":
			// 按 ohpm likes 排序
			result = p1.Likes < p2.Likes
		case "ohpmDownloads":
			// 按 ohpm downloads 排序
			result = p1.Downloads < p2.Downloads
		case "githubStars":
			// 按 github stars 排序
			result = p1.GithubBaseInfo.StargazersCount < p2.GithubBaseInfo.StargazersCount
		default:
			result = p1.Name < p2.Name
		}
		if isDesc {
			return !result
		}
		return result
	})
}

// 组装表格内容
//
// 参数:
//   - [packageInfoList]  信息列表
//   - [sortField]        排序字段 可选：name(default) | publishTime | ohpmLikes | ohpmDownloads | githubStars
//
// 返回值:
//   - markdown 表格内容
func assembleMarkdownTable(packageInfoList []PackageInfo, sortField string) string {
	markdownTableList := []MarkdownTable{}
	for _, value := range packageInfoList {
		var name, version, licenseName, publishTime, githubStars, ohpmLikes, ohpmDownloads, points, popularity, issues, pullRequests, contributors string
		switch value.Code {
		case 0:
			// 无法获取信息
			name = value.Name + " ⁉️"
		case 1:
			// 已获取信息
			// Base
			const ohpmLogo = "data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAABQAAAAUCAMAAAC6V+0/AAAA6lBMVEUAAABswm92x09tw2pCq+xhvItMsM9Qs8FhvIxhvI9Bq+1mvn5rwm9OssdqwnFhvI1FreJTtLdowHdMsM1lvoJvxGJJr9hQssJ6yUNeupdXtq1auKJlvoNCq+tFrONJr9ZlvoJzxVlGreFwxGFQssNauKJMscxeuphErOVlvoN6yUNDq+lIrtpTtLdov3lLsM9hvIxAqvJhvI1ErOZwxGB6yUNTtLhAqvF6yUNwxGJ6yUNlvoJeupdzxVlwxGB6yUNHrt1swm1swm1swmxXtq1Xtq1yxVtpwHZvw2RwxGFnv3tnv3t6yUN6yUPKo5kKAAAATnRSTlMABRQL+Ho1JiMeGxoRCKL+/Pz8+PPz8fHx8Ovk4dPOzszGxcKsqqCYh4F/fXp3d2xoZ2VhWlZRR0dBNTEvLiUhFvy9taGgmI+Nf2loaGciFjA1AAAAo0lEQVQY02MgDfCy6bqqqhtxIIuxq4kLSklL8svo8cDF2OSFVczYOW0MFIT4mKBiXEpi+rxgFre7kwlUUFtAB6aHkZERqlBWzgHM4GDV5GbyNGGw0LJnMGfRgCpjFfFmVlZk9pEwZTBkMYZqZrbmZLCzZWSyYsIqiKLdC6TdV8IU1SJLUTeQRShO4nEWtcRwPJ+jByMWb/LgChBE0LmAg45kAADNURSuaNgr4QAAAABJRU5ErkJggg=="
			const downloadIcon = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0icmdiYSgyNTUsMjU1LDI1NSwxKSI+PHBhdGggZD0iTTMgMTlIMjFWMjFIM1YxOVpNMTMgOUgyMEwxMiAxN0w0IDlIMTFWMUgxM1Y5WiI+PC9wYXRoPjwvc3ZnPg=="
			const popularityIcon = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0icmdiYSgyNTUsMjU1LDI1NSwxKSI+PHBhdGggZmlsbD0ibm9uZSIgZD0iTTAgMGgyNHYyNEgweiI+PC9wYXRoPjxwYXRoIGQ9Ik0xMiAyM0M3Ljg1Nzg2IDIzIDQuNSAxOS42NDIxIDQuNSAxNS41QzQuNSAxMy4zNDYyIDUuNDA3ODYgMTEuNDA0NSA2Ljg2MTc5IDEwLjAzNjZDOC4yMDQwMyA4Ljc3Mzc1IDExLjUgNi40OTk1MSAxMSAxLjVDMTcgNS41IDIwIDkuNSAxNCAxNS41QzE1IDE1LjUgMTYuNSAxNS41IDE5IDEzLjAyOTZDMTkuMjY5NyAxMy44MDMyIDE5LjUgMTQuNjM0NSAxOS41IDE1LjVDMTkuNSAxOS42NDIxIDE2LjE0MjEgMjMgMTIgMjNaIj48L3BhdGg+PC9zdmc+"
			const pointIcon = "data:image/svg+xml;base64,PHN2ZyB4bWxucz0iaHR0cDovL3d3dy53My5vcmcvMjAwMC9zdmciIHZpZXdCb3g9IjAgMCAyNCAyNCIgZmlsbD0icmdiYSgyNTUsMjU1LDI1NSwxKSI+PHBhdGggZD0iTTEuOTQ2MDcgOS4zMTU0M0MxLjQyMzUzIDkuMTQxMjUgMS40MTk0IDguODYwMjIgMS45NTY4MiA4LjY4MTA4TDIxLjA0MyAyLjMxOTAxQzIxLjU3MTUgMi4xNDI4NSAyMS44NzQ2IDIuNDM4NjYgMjEuNzI2NSAyLjk1Njk0TDE2LjI3MzMgMjIuMDQzMkMxNi4xMjIzIDIyLjU3MTYgMTUuODE3NyAyMi41OSAxNS41OTQ0IDIyLjA4NzZMMTEuOTk5OSAxNEwxNy45OTk5IDYuMDAwMDVMOS45OTk5MiAxMkwxLjk0NjA3IDkuMzE1NDNaIj48L3BhdGg+PC9zdmc+"

			name = "[" + value.Name + "](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			version = "v" + value.Version
			licenseName = "<strong>License:</strong> "
			if value.LicenseName != "" {
				licenseName += value.LicenseName
			} else {
				licenseName += "-"
			}
			publishTime = "<strong>PublishTime:</strong> " + timestampFormat(value.PublishTime)
			githubStars = ""
			ohpmLikes = "[![OHPM likes](https://img.shields.io/badge/" + strconv.Itoa(value.Likes) + "-_?style=social&logo=" + ohpmLogo + "&logoColor=168AFD&label=)](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			ohpmDownloads = "[![OHPM downloads](https://img.shields.io/badge/" + formatNumber(value.Downloads) + "-4AC51C?style=flat&logo=" + downloadIcon + ")](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			popularity = "[![OHPM popularity](https://img.shields.io/badge/" + formatNumber(value.Popularity) + "-4AC51C?style=flat&logo=" + popularityIcon + ")](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"

			pointsValue := float64(value.Points)
			maxPointsValue := float64(value.MaxPoints)
			pointsBackgroundColor := "4AC51C"
			if pointsValue < maxPointsValue {
				pointsBackgroundColor = "95C30D"
			}
			if pointsValue < maxPointsValue*0.5 {
				pointsBackgroundColor = "9FA226"
			}
			if pointsValue < maxPointsValue*0.2 {
				pointsBackgroundColor = "D6AE22"
			}
			if pointsValue < maxPointsValue*0.1 {
				pointsBackgroundColor = "D66049"
			}
			pointsText := strconv.Itoa(value.Points) + url.PathEscape("/") + strconv.Itoa(value.MaxPoints)
			points = "[![OHPM points](https://img.shields.io/badge/" + pointsText + "-" + pointsBackgroundColor + "?style=flat&logo=" + pointIcon + ")](https://ohpm.openharmony.cn/#/cn/detail/" + url.PathEscape(value.Name) + ")"
			issues = "-"
			pullRequests = "-"

			// Github
			if value.GithubUser != "" && value.GithubRepo != "" {
				githubURL := value.GithubUser + "/" + value.GithubRepo
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
						contributors += `<a href="` + githubContributorsInfoList[0].HtmlUrl + `"><img width="36px" src="` + getGithubAvatarUrl(githubContributorsInfoList[0].Id) + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
					case 2:
						contributors += `<tr align="center">`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[0].HtmlUrl + `"><img width="30px" src="` + getGithubAvatarUrl(githubContributorsInfoList[0].Id) + `" /></a>`
						contributors += `</td>`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[1].HtmlUrl + `"><img width="30px" src="` + getGithubAvatarUrl(githubContributorsInfoList[1].Id) + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
					case 3:
						contributors += `<tr align="center">`
						contributors += `<td colspan="2">`
						contributors += `<a href="` + githubContributorsInfoList[0].HtmlUrl + `"><img width="36px" src="` + getGithubAvatarUrl(githubContributorsInfoList[0].Id) + `" /></a>`
						contributors += `</td>`
						contributors += `</tr>`
						contributors += `<tr align="center">`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[1].HtmlUrl + `"><img width="30px" src="` + getGithubAvatarUrl(githubContributorsInfoList[1].Id) + `" /></a>`
						contributors += `</td>`
						contributors += `<td>`
						contributors += `<a href="` + githubContributorsInfoList[2].HtmlUrl + `"><img width="30px" src="` + getGithubAvatarUrl(githubContributorsInfoList[2].Id) + `" /></a>`
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
				Name:          name,
				Version:       version,
				Description:   value.Description,
				LicenseName:   licenseName,
				PublishTime:   publishTime,
				GithubStars:   githubStars,
				OhpmLikes:     ohpmLikes,
				OhpmDownloads: ohpmDownloads,
				Points:        points,
				Popularity:    popularity,
				Issues:        issues,
				PullRequests:  pullRequests,
				Contributors:  contributors,
			},
		)
	}

	markdown := ""
	markdown += "<sub>Sort by " + sortField + " | Total " + strconv.Itoa(len(markdownTableList)) + "</sub> \n\n" +
		"| <sub>Package</sub> | <sub>Stars/Likes</sub> | <sub>Downloads/Popularity / Points</sub> | <sub>Issues / Pull_requests</sub> | <sub>Contributors</sub> | \n" +
		"|--------------------|------------------------|------------------------------|-----------------------------------|:-----------------------:| \n"
	for _, value := range markdownTableList {
		markdown += "" +
			"| " + value.Name + " <sup><strong>" + value.Version + "</strong></sup> <br/> <sub>" + formatString(value.Description) + "</sub> <br/> <sub>" + value.LicenseName + "</sub> <br/> <sub>" + value.PublishTime + "</sub>" +
			" | " + value.GithubStars + " <br/> " + value.OhpmLikes +
			" | " + value.OhpmDownloads + " <br/> " + value.Popularity + " <br/> " + value.Points +
			" | " + value.Issues + " <br/> " + value.PullRequests +
			" | " + value.Contributors +
			" | \n"
	}
	return markdown
}

// 更新 Markdown 表格
//
// 识别：<!-- md:OHPMDashboard begin --><!-- md:OHPMDashboard end -->
//
// 参数:
//   - [filename] 更新的文件
//   - [markdown] 更新内容
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

	err = os.WriteFile(filename, newMd, 0644)
	if err != nil {
		return fmt.Errorf("📄❌ updateMarkdownTable: Error writing a file: %w", err)
	}
	fmt.Println("📄✅ updateMarkdownTable: Success")
	return nil
}

// 更新 Markdown Package 总数计数
//
// 识别：<!-- md:OHPMDashboard-total begin --><!-- md:OHPMDashboard-total end -->
//
// 参数:
//   - [filename] 更新的文件
//   - [total]    总数
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

	err = os.WriteFile(filename, newMd, 0644)
	if err != nil {
		return fmt.Errorf("📄❌ updateMarkdownPackageTotal: Error writing a file: %w", err)
	}
	fmt.Println("📄✅ updateMarkdownPackageTotal: Success")
	return nil
}

// 创建带超时的共享 HTTP Client。
//
// 复用同一个 Client 可共享连接池；
// Timeout 防止单个请求挂起拖垮整个流程。
func newHTTPClient() *http.Client {
	return &http.Client{Timeout: httpTimeout}
}

// concurrentMap 以 [concurrency] 为上限并发地将 fn 应用到每个 item，
// 结果按输入顺序写入返回切片（result[i] 对应 items[i]）。
//
// 任一 fn 返回错误即取消其余任务并返回首个错误。
// 由于每个 goroutine 只写入互不重叠的 result[i]，无需对结果加锁。
//
// 参数:
//   - [ctx]         上下文（用于取消与超时传播）
//   - [items]       输入切片
//   - [concurrency] 并发数上限
//   - [fn]          处理函数
//
// 返回值:
//   - [results]     结果切片（与 items 顺序一致）
//   - [error]       任一 fn 返回错误时非 nil
func concurrentMap[T any, R any](ctx context.Context, items []T, concurrency int, fn func(context.Context, T) (R, error)) ([]R, error) {
	results := make([]R, len(items))
	if len(items) == 0 {
		return results, nil
	}
	if concurrency < 1 {
		concurrency = 1
	}

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		waitGroup sync.WaitGroup
		mutex     sync.Mutex
		firstErr  error
	)
	semaphore := make(chan struct{}, concurrency)

	for i, item := range items {
		waitGroup.Add(1)
		go func(i int, item T) {
			defer waitGroup.Done()

			// 并发限流，若已取消则直接放弃
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()

			r, err := fn(ctx, item)
			if err != nil {
				mutex.Lock()
				if firstErr == nil {
					firstErr = err
					cancel() // fail-fast：取消其余在途/未启动任务
				}
				mutex.Unlock()
				return
			}
			results[i] = r
		}(i, item)
	}
	waitGroup.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

// 带重试的 HTTP GET 请求
//
// 对传输层错误、429 (Too Many Requests)、403 (限流/鉴权)、5xx 进行指数退避重试，
// 最多尝试 [maxAttempts] 次；退避期间响应 ctx 取消。
//
// 仅负责传输 + 重试瞬时故障，状态码的业务语义（如 404 的含义）
// 由调用方根据返回的 status 自行解释。
//
// 参数:
//   - [ctx]     上下文（用于取消与超时传播）
//   - [client]  共享 HTTP Client
//   - [rawURL]  请求地址
//   - [headers] 附加请求头（可为 nil）
//
// 返回值:
//   - 响应体
//   - HTTP 状态码
//   - 错误（传输层彻底失败或重试耗尽时非 nil）
func httpGetWithRetry(ctx context.Context, client *http.Client, rawURL string, headers map[string]string) ([]byte, int, error) {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// 退避（首次不等待）：500ms, 1s, 2s ...
		if attempt > 1 {
			delay := retryBaseDelay * time.Duration(1<<(attempt-2))
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(delay):
			}
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, 0, err // 构造请求失败不可恢复
		}
		for key, value := range headers {
			req.Header.Set(key, value)
		}

		res, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, 0, ctx.Err() // 已取消则立即返回
			}
			lastErr = err
			continue
		}

		body, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		status := res.StatusCode

		if readErr != nil {
			if ctx.Err() != nil {
				return nil, status, ctx.Err()
			}
			lastErr = readErr
			continue
		}

		// 可重试的状态码：限流与服务端错误
		if status == http.StatusTooManyRequests || status == http.StatusForbidden || status >= 500 {
			lastErr = fmt.Errorf("unexpected status %d", status)
			continue
		}

		// 成功或不可重试的状态码（2xx、404 等），交由调用方判断
		return body, status, nil
	}
	return nil, 0, fmt.Errorf("After %d attempts: %w", maxAttempts, lastErr)
}

// 由于直接获取 GithubContributorsInfo.AvatarUrl 有可能会是私有头像地址，
// 暂时固定头像地址。
//
// 参数:
//   - [githubId] Github ID
func getGithubAvatarUrl(githubId int) string {
	return "https://avatars.githubusercontent.com/u/" + strconv.Itoa(githubId) + "?v=4"
}

// 格式化字符串（防止 markdown 格式错乱）
//
// 参数:
//   - [v] 需要格式化的字符
//
// 返回值:
//   - 格式化后的字符
func formatString(v string) string {
	value := v
	value = strings.ReplaceAll(value, "\n", " ")
	value = strings.ReplaceAll(value, "|", "丨")
	return value
}

// 格式化下载数量（便于展示）
//
// 参数:
//   - [num] 需要格式化的数量
//
// 返回值:
//   - 格式化后的数量字符
func formatNumber(num int) string {
	var formatted, suffix string
	if num >= 1000000 {
		formatted = fmt.Sprintf("%.2f", float64(num)/1000000)
		suffix = "M"
	} else if num >= 1000 {
		formatted = fmt.Sprintf("%.2f", float64(num)/1000)
		suffix = "k"
	} else {
		return strconv.Itoa(num)
	}

	// 去掉多余的 0 和小数点
	formatted = strings.TrimRight(strings.TrimRight(formatted, "0"), ".")
	return formatted + suffix
}

// 去重并保持首次出现的顺序，同时去除首尾空白与空字符串
func removeDuplicates(arr []string) []string {
	seen := make(map[string]bool, len(arr))
	result := []string{}
	for _, v := range arr {
		v = strings.TrimSpace(v)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		result = append(result, v)
	}
	return result
}

func timestampFormat(millisecondTimestamp int) string {
	timestamp := int64(millisecondTimestamp)
	seconds := timestamp / 1000
	nanoseconds := (timestamp % 1000) * int64(time.Millisecond)
	t := time.Unix(seconds, nanoseconds)
	return t.Format(time.RFC3339)
}
