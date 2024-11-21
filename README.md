# ohpm-dashboard

Example: [Example.md](Example.md)

## Setup 💻

1.Add comments to the place where you want to update in the markdown file.

* Table

```
<!-- md:OHPMDashboard begin --><!-- md:OHPMDashboard end -->
```

* Package total

```
<!-- md:OHPMDashboard-total begin --><!-- md:OHPMDashboard-total end -->
```

2.Enable read/write permissions

(recommend) If you use a `Personal access token`:
> e.g. github_token: ${{ secrets.xxxxxx }} (ghp_xxxxx)  
> Create a token (enable repo permissions) https://github.com/settings/tokens/

If you use the current repo's token:
> e.g. github_token: ${{ secrets.GITHUB_TOKEN }}  
> https://docs.github.com/en/actions/security-guides/automatic-token-authentication  
> Current repo's settings: Settings -> Actions -> General -> Workflow permissions -> Read and write permissions 

3.Edit the settings in `.github/workflows/ohpm-dashboard.yml`

```yaml
...

jobs:
  ohpm-dashboard-job:
    runs-on: ubuntu-latest
    name: ohpm-dashboard
    steps:
      - name: run ohpm-dashboard
        uses: AmosHuKe/ohpm-dashboard@v1
        with:
          github_token: ${{ Personal access token }} or ${{ secrets.GITHUB_TOKEN }}
          github_repo: "https://github.com/AmosHuKe/ohpm-dashboard"
          filename: "Example.md"
          publisher_list: "6542179b6dad4e55f6635764" # https://ohpm.openharmony.cn/#/cn/publisher/6542179b6dad4e55f6635764
          package_list: "@candies/extended_text,@bb/xx,@cc/xx"
          sort_field: "publishTime"
          sort_mode: "asc"

...
```

| Setting | Default | Value | Description |  
|---------|---------|-------|-------------|
| github_token <sup>`required`</sup> | - | - | Github Token with repo permissions |
| github_repo <sup>`required`</sup> | - | - | Github repo to be manipulated |
| commit_message | docs(ohpm-dashboard): ohpm-dashboard has updated readme | - | Commit message |
| committer_username | github-actions[bot] | - | Committer username |
| committer_email | 41898282+github-actions[bot]@users.noreply.github.com | - | Committer email |
| filename | README.md | - | Markdown file <br/> e.g. "README.md" "test/test.md" |
| publisher_list | - | - | Publisher ID (`,` split) <br/> https://ohpm.openharmony.cn/#/cn/publisher/6542179b6dad4e55f6635764 <br/> e.g. "6542179b6dad4e55f6635764,xxx,xxx" |
| package_list | - | - | Package name (`,` split) <br/> e.g. "@candies/extended_text,@bb/xx,@cc/xx" |
| sort_field | name | name, publishTime, ohpmLikes, githubStars | Sort field |
| sort_mode | asc | asc, desc | Sort mode |

## Tips 💡

- ⁉️: Package not found
- `publisher_list` and `package_list` are merged
- The `Github link` is resolved by the `Homepage`, `Repository` of `ohpm.openharmony.cn`

Thanks [Shields](https://github.com/badges/shields).

## License 📄

Open sourced under the [Apache-2.0](LICENSE).

© AmosHuKe