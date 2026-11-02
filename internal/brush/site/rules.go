// Copyright (c) 2026, vertex-qui contributors.
// SPDX-License-Identifier: GPL-2.0-or-later

package site

import (
	"bytes"
	"errors"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// rule reads one site's details page. The per-site rules follow the original
// Vertex's scrape.js; every other host gets the generic NexusPHP rule.
type rule struct {
	name string
	free func(doc *html.Node) (bool, error)
	hr   func(doc *html.Node, page []byte) bool
}

var genericRule = rule{name: "nexusphp", free: freeInTitle, hr: hrImage}

var hostRules = func() map[string]rule {
	rules := map[string]rule{}
	add := func(r rule, hosts ...string) {
		for _, h := range hosts {
			rules[h] = r
		}
	}
	add(rule{name: "opencd", free: freeOpencd, hr: hrImage}, "open.cd", "www.open.cd")
	add(rule{name: "totheglory", free: freeTTG, hr: hrTTG}, "totheglory.im")
	add(rule{name: "u2", free: freeU2, hr: hrImage}, "u2.dmhy.org")
	add(rule{name: "bold-font", free: freeBoldFont, hr: hrImage}, "club.hares.top", "pt.sjtu.edu.cn")
	add(rule{name: "byr", free: freeBYR, hr: hrImage}, "byr.pt")
	add(rule{name: "hhanclub", free: freeHHan, hr: hrImage}, "hhanclub.net")
	add(rule{name: "hudbt", free: freeHUDBT, hr: hrImage}, "zeus.hamsters.space", "hudbt.hust.edu.cn")
	add(rule{name: "hdcity", free: freeHDCity, hr: hrImage}, "hdcity.leniter.org", "hdcity.city")
	add(rule{name: "chdbits", free: freeInTitle, hr: hrCHDBits}, "chdbits.co", "ptchdbits.co")
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
// A page without #top cannot be read, so it is an error, never "not free".
func freeInTitle(doc *html.Node) (bool, error) {
	top := findByID(doc, "top")
	if top == nil {
		return false, errors.New("details page has no #top title")
	}
	return hasClass(top, titleFreeClasses...), nil
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

func hrImage(doc *html.Node, _ []byte) bool {
	return first(doc, func(n *html.Node) bool { return isTag(n, "img") && attr(n, "class") == "hitandrun" }) != nil
}

func hrTTG(doc *html.Node, _ []byte) bool {
	return first(doc, func(n *html.Node) bool {
		return isTag(n, "img") && attr(n, "src") == "/pic/hit_run.gif" && attr(n, "alt") == "Hit & Run"
	}) != nil
}

// hrCHDBits reads the H&R row of the details table, which CHDBits writes as text.
func hrCHDBits(_ *html.Node, page []byte) bool {
	return bytes.Contains(page, []byte("<b>H&amp;R:&nbsp;</b>5day</td></tr>")) ||
		bytes.Contains(page, []byte("<b>H&amp;R:&nbsp;</b>3day</td></tr>"))
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
