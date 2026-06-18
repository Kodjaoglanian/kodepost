package main

import (
	"bytes"
	"flag"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

// Post representa um artigo do blog.
type Post struct {
	Title    string
	Date     time.Time
	Tags     []string
	Slug     string
	Summary  string
	Content  template.HTML
	RawPath  string
	Featured bool
}

// Page representa uma página estática (ex: Sobre).
type Page struct {
	Title   string
	Slug    string
	Content template.HTML
}

// TagInfo agrupa posts por tag.
type TagInfo struct {
	Name  string
	Posts []Post
}

// PageData é injetado no template layout.html.
type PageData struct {
	IsHome      bool
	IsPage      bool
	IsTags      bool
	Title       string
	Description string
	Canonical   string
	Posts       []Post
	Post        *Post
	Page        *Page
	Groups      []YearGroup
	Tags        []TagInfo
	AllTags     []string // para a sidebar/nuvem
}

// YearGroup agrupa posts por ano/mês.
type YearGroup struct {
	Year   int
	Months []MonthGroup
}

type MonthGroup struct {
	Month     time.Month
	MonthName string
	Posts     []Post
}

func main() {
	if len(os.Args) > 1 && os.Args[1] == "serve" {
		if err := serve(); err != nil {
			fmt.Fprintf(os.Stderr, "erro: %v\n", err)
			os.Exit(1)
		}
		return
	}
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "erro: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("Site gerado com sucesso em public/")
}

func serve() error {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	port := fs.String("port", "8765", "porta do servidor HTTP")
	_ = fs.Parse(os.Args[2:])

	if _, err := os.Stat("public/index.html"); os.IsNotExist(err) {
		fmt.Println("Gerando site...")
		if err := run(); err != nil {
			return err
		}
	} else {
		fmt.Println("Usando public/ pré-gerado.")
	}

	addr := ":" + *port
	fmt.Printf("Servindo em http://localhost%s\n", addr)
	fmt.Println("Pressione Ctrl+C para parar.")

	fsrv := http.FileServer(http.Dir("public"))
	return http.ListenAndServe(addr, fsrv)
}

func run() error {
	if err := os.RemoveAll("public"); err != nil {
		return fmt.Errorf("falha ao limpar public/: %w", err)
	}
	if err := os.MkdirAll("public", 0755); err != nil {
		return fmt.Errorf("falha ao criar public/: %w", err)
	}

	// Carregar posts e páginas
	posts, err := loadPosts("posts")
	if err != nil {
		return fmt.Errorf("falha ao carregar posts: %w", err)
	}
	pages, err := loadPages("pages")
	if err != nil {
		return fmt.Errorf("falha ao carregar páginas: %w", err)
	}

	// Ordenar posts cronologicamente decrescente (estável para mesma data)
	sort.SliceStable(posts, func(i, j int) bool {
		return posts[i].Date.After(posts[j].Date)
	})

	// Copiar imagens
	if err := copyImages("assets/images", "public/assets/images"); err != nil {
		return fmt.Errorf("falha ao copiar imagens: %w", err)
	}

	// Extrair todas as tags únicas
	allTags := extractAllTags(posts)

	// Gerar index.html (home)
	if err := generateIndex(posts, allTags, "templates/layout.html", "public/index.html"); err != nil {
		return fmt.Errorf("falha ao gerar index: %w", err)
	}

	// Gerar páginas individuais de posts
	if err := generatePosts(posts, allTags, "templates/layout.html", "public"); err != nil {
		return fmt.Errorf("falha ao gerar posts: %w", err)
	}

	// Gerar páginas estáticas
	if err := generatePages(pages, posts, allTags, "templates/layout.html", "public"); err != nil {
		return fmt.Errorf("falha ao gerar páginas: %w", err)
	}

	// Gerar página de tags
	if err := generateTagsPage(posts, allTags, "templates/layout.html", "public/tags/index.html"); err != nil {
		return fmt.Errorf("falha ao gerar tags: %w", err)
	}

	// Gerar feed RSS
	if err := generateRSS(posts, "public/rss.xml"); err != nil {
		return fmt.Errorf("falha ao gerar rss: %w", err)
	}

	// Gerar sitemap
	if err := generateSitemap(posts, pages, "public/sitemap.xml"); err != nil {
		return fmt.Errorf("falha ao gerar sitemap: %w", err)
	}

	return nil
}

func loadPosts(dir string) ([]Post, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var posts []Post
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		post, err := parsePost(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		posts = append(posts, post)
	}
	return posts, nil
}

func parsePost(path string) (Post, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Post{}, err
	}
	var post Post
	post.RawPath = path

	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return Post{}, fmt.Errorf("front matter ausente")
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return Post{}, fmt.Errorf("front matter mal formatado")
	}

	var meta struct {
		Title    string   `yaml:"title"`
		Date     string   `yaml:"date"`
		Tags     []string `yaml:"tags"`
		Summary  string   `yaml:"summary"`
		Featured bool     `yaml:"featured"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return Post{}, fmt.Errorf("yaml inválido: %w", err)
	}

	post.Title = meta.Title
	post.Summary = meta.Summary
	post.Tags = meta.Tags
	post.Featured = meta.Featured

	post.Date, err = time.Parse("2006-01-02", meta.Date)
	if err != nil {
		post.Date, err = time.Parse("2006-01-02 15:04:05", meta.Date)
		if err != nil {
			post.Date, err = time.Parse(time.RFC3339, meta.Date)
			if err != nil {
				return Post{}, fmt.Errorf("data inválida '%s': %w", meta.Date, err)
			}
		}
	}

	base := filepath.Base(path)
	post.Slug = strings.TrimSuffix(base, filepath.Ext(base))

	mdBody := strings.TrimSpace(parts[2])
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(mdBody), &buf); err != nil {
		return Post{}, fmt.Errorf("falha ao converter markdown: %w", err)
	}
	post.Content = template.HTML(buf.String())

	return post, nil
}

func loadPages(dir string) ([]Page, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pages []Page
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		page, err := parsePage(path)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		pages = append(pages, page)
	}
	return pages, nil
}

func parsePage(path string) (Page, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Page{}, err
	}
	content := string(data)
	if !strings.HasPrefix(content, "---") {
		return Page{}, fmt.Errorf("front matter ausente")
	}
	parts := strings.SplitN(content, "---", 3)
	if len(parts) < 3 {
		return Page{}, fmt.Errorf("front matter mal formatado")
	}

	var meta struct {
		Title string `yaml:"title"`
	}
	if err := yaml.Unmarshal([]byte(parts[1]), &meta); err != nil {
		return Page{}, fmt.Errorf("yaml inválido: %w", err)
	}

	base := filepath.Base(path)
	slug := strings.TrimSuffix(base, filepath.Ext(base))

	mdBody := strings.TrimSpace(parts[2])
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(mdBody), &buf); err != nil {
		return Page{}, fmt.Errorf("falha ao converter markdown: %w", err)
	}

	return Page{
		Title:   meta.Title,
		Slug:    slug,
		Content: template.HTML(buf.String()),
	}, nil
}

func extractAllTags(posts []Post) []string {
	tagSet := make(map[string]bool)
	for _, p := range posts {
		for _, t := range p.Tags {
			tagSet[t] = true
		}
	}
	var tags []string
	for t := range tagSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}

func generateIndex(posts []Post, allTags []string, tmplPath, outPath string) error {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}

	// Destaques: posts com featured=true, ou os 3 mais recentes se nenhum
	var featured []Post
	for _, p := range posts {
		if p.Featured {
			featured = append(featured, p)
		}
	}
	if len(featured) == 0 && len(posts) > 0 {
		end := 3
		if len(posts) < end {
			end = len(posts)
		}
		featured = posts[:end]
	}

	groups := buildTimelineGroups(posts)
	data := PageData{
		IsHome:      true,
		Title:       "KodePost",
		Description: "Pensamentos sobre arquitetura de software, evolução de carreira e tecnologia.",
		Canonical:   "https://kodepost.com/",
		Posts:       featured, // destaques
		Groups:      groups,
		AllTags:     allTags,
	}

	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

func generatePosts(posts []Post, allTags []string, tmplPath, outDir string) error {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}
	for i := range posts {
		postDir := filepath.Join(outDir, posts[i].Slug)
		if err := os.MkdirAll(postDir, 0755); err != nil {
			return err
		}
		desc := posts[i].Summary
		if desc == "" {
			desc = "Artigo de Bruno Kodjaoglanian sobre tecnologia, arquitetura de software e carreira."
		}
		data := PageData{
			IsHome:      false,
			Title:       posts[i].Title,
			Description: desc,
			Canonical:   "https://kodepost.com/" + posts[i].Slug + "/",
			Posts:       posts,
			Post:        &posts[i],
			AllTags:     allTags,
		}
		outPath := filepath.Join(postDir, "index.html")
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		if err := tmpl.Execute(f, data); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

func generatePages(pages []Page, posts []Post, allTags []string, tmplPath, outDir string) error {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}
	for i := range pages {
		pageDir := filepath.Join(outDir, pages[i].Slug)
		if err := os.MkdirAll(pageDir, 0755); err != nil {
			return err
		}
		data := PageData{
			IsPage:      true,
			Title:       pages[i].Title,
			Description: pages[i].Title + " — KodePost.",
			Canonical:   "https://kodepost.com/" + pages[i].Slug + "/",
			Page:        &pages[i],
			Posts:       posts,
			AllTags:     allTags,
		}
		outPath := filepath.Join(pageDir, "index.html")
		f, err := os.Create(outPath)
		if err != nil {
			return err
		}
		if err := tmpl.Execute(f, data); err != nil {
			f.Close()
			return err
		}
		f.Close()
	}
	return nil
}

func generateTagsPage(posts []Post, allTags []string, tmplPath, outPath string) error {
	tmpl, err := template.ParseFiles(tmplPath)
	if err != nil {
		return err
	}

	// Agrupar posts por tag
	tagMap := make(map[string][]Post)
	for _, p := range posts {
		for _, t := range p.Tags {
			tagMap[strings.ToLower(t)] = append(tagMap[strings.ToLower(t)], p)
		}
	}

	var tagInfos []TagInfo
	for _, t := range allTags {
		tagInfos = append(tagInfos, TagInfo{
			Name:  t,
			Posts: tagMap[strings.ToLower(t)],
		})
	}

	data := PageData{
		IsTags:      true,
		Title:       "Tags — KodePost",
		Description: "Navegue por todos os artigos do KodePost agrupados por tag.",
		Canonical:   "https://kodepost.com/tags/",
		Tags:        tagInfos,
		AllTags:     allTags,
		Posts:       posts,
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return err
	}
	defer f.Close()
	return tmpl.Execute(f, data)
}

func buildTimelineGroups(posts []Post) []YearGroup {
	yearMap := make(map[int]map[time.Month][]Post)
	for _, p := range posts {
		y, m := p.Date.Year(), p.Date.Month()
		if yearMap[y] == nil {
			yearMap[y] = make(map[time.Month][]Post)
		}
		yearMap[y][m] = append(yearMap[y][m], p)
	}

	var years []int
	for y := range yearMap {
		years = append(years, y)
	}
	sort.Sort(sort.Reverse(sort.IntSlice(years)))

	var groups []YearGroup
	for _, y := range years {
		var months []time.Month
		for m := range yearMap[y] {
			months = append(months, m)
		}
		sort.Slice(months, func(i, j int) bool {
			return months[i] > months[j]
		})
		var mg []MonthGroup
		for _, m := range months {
			mp := yearMap[y][m]
			// Garantir ordem decrescente dentro do mês (estável para mesma data)
			sort.SliceStable(mp, func(i, j int) bool {
				return mp[i].Date.After(mp[j].Date)
			})
			mg = append(mg, MonthGroup{
				Month:     m,
				MonthName: monthNamePT(m),
				Posts:     mp,
			})
		}
		groups = append(groups, YearGroup{Year: y, Months: mg})
	}
	return groups
}

func monthNamePT(m time.Month) string {
	names := []string{
		"", "Janeiro", "Fevereiro", "Março", "Abril", "Maio", "Junho",
		"Julho", "Agosto", "Setembro", "Outubro", "Novembro", "Dezembro",
	}
	return names[m]
}

func copyImages(srcDir, dstDir string) error {
	if _, err := os.Stat(srcDir); os.IsNotExist(err) {
		return nil
	}
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dstDir, rel)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		dst, err := os.Create(dstPath)
		if err != nil {
			return err
		}
		_, err = io.Copy(dst, src)
		dst.Close()
		return err
	})
}

// extractTagStrings converte []Post tags para []string únicas
func extractTagStrings(posts []Post) []string {
	return extractAllTags(posts)
}

func generateRSS(posts []Post, outPath string) error {
	items := ""
	for _, p := range posts {
		desc := p.Summary
		if desc == "" {
			desc = p.Title
		}
		items += fmt.Sprintf(
			"<item>\n<title>%s</title>\n<link>https://kodepost.com/%s/</link>\n<pubDate>%s</pubDate>\n<guid>https://kodepost.com/%s/</guid>\n<description><![CDATA[%s]]></description>\n</item>\n",
			escapeXML(p.Title), p.Slug, p.Date.Format(time.RFC1123), p.Slug, desc,
		)
	}

	rss := `<?xml version="1.0" encoding="UTF-8"?>
<rss version="2.0">
<channel>
<title>KodePost</title>
<link>https://kodepost.com</link>
<description>Pensamentos sobre arquitetura de software, evolução de carreira e tecnologia.</description>
<language>pt-BR</language>
<lastBuildDate>` + time.Now().Format(time.RFC1123) + `</lastBuildDate>
` + items + `</channel>
</rss>`

	return os.WriteFile(outPath, []byte(rss), 0644)
}

func generateSitemap(posts []Post, pages []Page, outPath string) error {
	urls := []string{
		fmt.Sprintf("<url><loc>https://kodepost.com/</loc><priority>1.0</priority></url>"),
		fmt.Sprintf("<url><loc>https://kodepost.com/tags/</loc><priority>0.5</priority></url>"),
	}
	for _, p := range posts {
		urls = append(urls, fmt.Sprintf("<url><loc>https://kodepost.com/%s/</loc><lastmod>%s</lastmod><priority>0.8</priority></url>", p.Slug, p.Date.Format("2006-01-02")))
	}
	for _, p := range pages {
		urls = append(urls, fmt.Sprintf("<url><loc>https://kodepost.com/%s/</loc><priority>0.6</priority></url>", p.Slug))
	}

	sitemap := `<?xml version="1.0" encoding="UTF-8"?>
<urlset xmlns="http://www.sitemaps.org/schemas/sitemap/0.9">
` + strings.Join(urls, "\n") + `
</urlset>`
	return os.WriteFile(outPath, []byte(sitemap), 0644)
}

func escapeXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	s = strings.ReplaceAll(s, "\"", "&quot;")
	s = strings.ReplaceAll(s, "'", "&apos;")
	return s
}
