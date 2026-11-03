// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"errors"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// rule reads one site's details page. The per-site rules follow the original
// Vertex's scrape.js; every other host gets the generic NexusPHP free rule. Only
// the hosts Vertex reads H&R on have an H&R rule: on any other the H&R markers
// are unknown, so a page without one proves nothing, and hr is nil.
type rule struct {
	name string
	free func(doc *html.Node) (bool, error)
	hr   func(doc *html.Node) bool
	// login is the text a logged-in page always has; empty means "userdetails".
	login string
}

var genericRule = rule{name: "nexusphp", free: freeInTitle}

var hostRules = func() map[string]rule {
	rules := map[string]rule{}
	add := func(r rule, hosts ...string) {
		for _, h := range hosts {
			rules[h] = r
		}
	}
	add(rule{name: "opencd", free: freeOpencd}, "open.cd", "www.open.cd")
	add(rule{name: "totheglory", free: freeTTG, hr: hrTTG}, "totheglory.im")
	add(rule{name: "u2", free: freeU2}, "u2.dmhy.org")
	add(rule{name: "bold-font", free: freeBoldFont}, "club.hares.top", "pt.sjtu.edu.cn")
	add(rule{name: "byr", free: freeBYR}, "byr.pt")
	add(rule{name: "hhanclub", free: freeHHan, hr: hrImage}, "hhanclub.net")
	add(rule{name: "hudbt", free: freeHUDBT}, "zeus.hamsters.space", "hudbt.hust.edu.cn")
	add(rule{name: "hdcity", free: freeHDCity}, "hdcity.leniter.org", "hdcity.city")
	add(rule{name: "hdarea", free: freeInTitle}, "www.hdarea.co", "www.hdarea.club", "hdarea.club")
	add(rule{name: "hdbits", free: freeHDBits}, "hdbits.org")
	add(rule{name: "bitporn", free: freeBitPorn}, "bitporn.eu")
	add(rule{name: "luminance", free: freeLuminance, login: "nav_userinfo"},
		"www.empornium.is", "www.empornium.sx", "www.pixelcove.me", "www.cathode-ray.tube")
	add(rule{name: "chdbits", free: freeInTitle, hr: hrCHDBits}, "chdbits.co", "ptchdbits.co")
	// The NexusPHP sites Vertex reads H&R on, as the hitandrun image.
	add(rule{name: "nexusphp", free: freeInTitle, hr: hrImage},
		"www.hddolby.com", "hdhome.org", "ourbits.club", "piggo.me", "sharkpt.net", "audiences.me")
	return rules
}()

func ruleFor(host string) rule {
	if r, ok := hostRules[strings.ToLower(host)]; ok {
		return r
	}
	return genericRule
}

var (
	titleFreeClasses = []string{"free", "twoupfree", "pro_free", "pro_free2up"}
	fontFreeClasses  = []string{"free", "twoupfree"}
	iconFreeClasses  = []string{"pro_free", "pro_free2up"}
)

// freeInTitle looks for a promotion class only inside the title (#top), because the
// rest of the page can carry the same classes for other torrents or site notices.
// Vertex reads only the title's first classed font or span, which a tag such as
// 热门 placed before the promotion hides; any element of the title counts here.
// A page without #top cannot be read, so it is an error, never "not free".
func freeInTitle(doc *html.Node) (bool, error) {
	top := findByID(doc, "top")
	if top == nil {
		return false, errors.New("details page has no #top title")
	}
	return first(top, func(n *html.Node) bool { return hasClassToken(n, titleFreeClasses...) }) != nil, nil
}

func freeHDBits(doc *html.Node) (bool, error) {
	return first(doc, func(n *html.Node) bool { return isTag(n, "span") && attr(n, "class") == "tag freeleech" }) != nil, nil
}

func freeBitPorn(doc *html.Node) (bool, error) {
	return first(doc, func(n *html.Node) bool {
		if !isTag(n, "span") || !hasClassToken(n, "stic2") {
			return false
		}
		t := strings.TrimSpace(text(n))
		return t == "2X Free" || t == "Free"
	}) != nil, nil
}

func freeLuminance(doc *html.Node) (bool, error) {
	return first(doc, func(n *html.Node) bool { return isTag(n, "img") && attr(n, "alt") == "Freeleech" }) != nil, nil
}

// freeOpencd is the first icon in div.title: div[class=title] img[class].
func freeOpencd(doc *html.Node) (bool, error) {
	img := first(doc, func(n *html.Node) bool {
		return isTag(n, "img") && attr(n, "class") != "" &&
			ancestor(n, func(a *html.Node) bool { return isTag(a, "div") && attr(a, "class") == "title" })
	})
	return img != nil && slices.Contains(iconFreeClasses, attr(img, "class")), nil
}

func freeTTG(doc *html.Node) (bool, error) {
	return first(doc, func(n *html.Node) bool {
		return isTag(n, "img") && attr(n, "src") == "/pic/ico_free.gif" && attr(n, "class") == "topic"
	}) != nil, nil
}

// freeU2 takes a free icon in the info cell, or a download ratio of 0.00X next to the
// down arrow, which is how U2 shows custom promotions.
func freeU2(doc *html.Node) (bool, error) {
	inInfo := func(n *html.Node) bool {
		return ancestor(n, func(a *html.Node) bool { return isTag(a, "td") && attr(a, "valign") == "top" })
	}
	if first(doc, func(n *html.Node) bool {
		return isTag(n, "img") && slices.Contains(iconFreeClasses, attr(n, "class")) && inInfo(n)
	}) != nil {
		return true, nil
	}
	arrow := first(doc, func(n *html.Node) bool { return isTag(n, "img") && attr(n, "class") == "arrowdown" && inInfo(n) })
	if arrow == nil {
		return false, nil
	}
	next := arrow.NextSibling
	for next != nil && next.Type == html.TextNode && strings.TrimSpace(next.Data) == "" {
		next = next.NextSibling
	}
	return next != nil && next.Type == html.ElementNode && strings.TrimSpace(text(next)) == "0.00X", nil
}

// freeBoldFont is the first classed font inside a bold tag: b font[class].
func freeBoldFont(doc *html.Node) (bool, error) {
	font := first(doc, func(n *html.Node) bool {
		return isTag(n, "font") && attr(n, "class") != "" && ancestor(n, func(a *html.Node) bool { return isTag(a, "b") })
	})
	return font != nil && slices.Contains(fontFreeClasses, attr(font, "class")), nil
}

// freeBYR is the first .free inside #share, whose class must be exactly free.
func freeBYR(doc *html.Node) (bool, error) {
	share := findByID(doc, "share")
	if share == nil {
		return false, nil
	}
	el := first(share, func(n *html.Node) bool { return n != share && hasClassToken(n, "free") })
	return el != nil && slices.Contains(fontFreeClasses, attr(el, "class")), nil
}

func freeHHan(doc *html.Node) (bool, error) {
	tag := first(doc, func(n *html.Node) bool { return hasClassToken(n, "promotion-tag") })
	return tag != nil && strings.Contains(attr(tag, "class"), "free"), nil
}

func freeHUDBT(doc *html.Node) (bool, error) {
	return first(doc, func(n *html.Node) bool {
		return isTag(n, "img") && slices.Contains(fontFreeClasses, attr(n, "class")) &&
			ancestor(n, func(a *html.Node) bool { return isTag(a, "div") && attr(a, "class") == "minor-list" })
	}) != nil, nil
}

func freeHDCity(doc *html.Node) (bool, error) {
	return first(doc, func(n *html.Node) bool { return isTag(n, "font") && hasClassToken(n, fontFreeClasses...) }) != nil, nil
}

func hrImage(doc *html.Node) bool {
	return first(doc, func(n *html.Node) bool { return isTag(n, "img") && attr(n, "class") == "hitandrun" }) != nil
}

func hrTTG(doc *html.Node) bool {
	return first(doc, func(n *html.Node) bool {
		return isTag(n, "img") && attr(n, "src") == "/pic/hit_run.gif" && attr(n, "alt") == "Hit & Run"
	}) != nil
}

// hrCHDBits reads the H&R field CHDBits writes as text at the end of the basic info
// row: <b>H&R:&nbsp;</b>3day or 5day. Vertex matched that string in the page as
// the browser re-serializes it, so the raw source, whose escaping and whitespace
// differ, is not compared byte for byte; the parsed nodes are read instead.
func hrCHDBits(doc *html.Node) bool {
	return first(doc, func(n *html.Node) bool {
		if !isTag(n, "b") || squash(text(n)) != "H&R:" {
			return false
		}
		days := n.NextSibling
		if days == nil || days.Type != html.TextNode {
			return false
		}
		if d := squash(days.Data); d != "3day" && d != "5day" {
			return false
		}
		// Only whitespace may follow it up to the end of the row.
		for after := days.NextSibling; after != nil; after = after.NextSibling {
			if after.Type != html.TextNode || squash(after.Data) != "" {
				return false
			}
		}
		return isTag(n.Parent, "td")
	}) != nil
}

// squash trims a text, no-break spaces included.
func squash(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, " ", " "))
}

// first returns the first node in document order that matches, n included.
func first(n *html.Node, match func(*html.Node) bool) *html.Node {
	if n.Type == html.ElementNode && match(n) {
		return n
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if found := first(c, match); found != nil {
			return found
		}
	}
	return nil
}

func ancestor(n *html.Node, match func(*html.Node) bool) bool {
	for p := n.Parent; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && match(p) {
			return true
		}
	}
	return false
}

func isTag(n *html.Node, tag string) bool {
	return n.Type == html.ElementNode && n.Data == tag
}

// hasClassToken reports whether the node itself has one of the classes.
func hasClassToken(n *html.Node, classes ...string) bool {
	if n.Type != html.ElementNode {
		return false
	}
	for _, have := range strings.Fields(attr(n, "class")) {
		if slices.Contains(classes, have) {
			return true
		}
	}
	return false
}

func text(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			b.WriteString(n.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}

// RuleName names how brush reads the host's torrents: a site rule, the generic
// NexusPHP rule, or M-Team's API.
func RuleName(host string) string {
	if isMTeam(host) {
		return "m-team"
	}
	if isColapt(host) {
		return "colapt"
	}
	return ruleFor(host).name
}
