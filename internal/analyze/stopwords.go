package analyze

// builtinStopwords 内置英语虚词 + 域名资讯套话噪点。
// 用户仍可通过 config.yaml / data/stopwords.txt 追加。
var builtinStopwords = map[string]bool{
	// 虚词 / 代词 / 助动词
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"if": true, "as": true, "at": true, "by": true, "for": true, "from": true,
	"in": true, "into": true, "of": true, "on": true, "to": true, "with": true,
	"without": true, "within": true, "about": true, "after": true, "before": true,
	"over": true, "under": true, "above": true, "below": true, "between": true,
	"through": true, "during": true, "against": true, "among": true, "across": true,
	"up": true, "down": true, "out": true, "off": true, "again": true, "further": true,
	"then": true, "once": true, "here": true, "there": true, "when": true, "where": true,
	"why": true, "how": true, "all": true, "any": true, "both": true, "each": true,
	"few": true, "more": true, "most": true, "other": true, "some": true, "such": true,
	"no": true, "nor": true, "not": true, "only": true, "own": true, "same": true,
	"so": true, "than": true, "too": true, "very": true, "just": true, "also": true,
	"can": true, "will": true, "shall": true, "should": true, "would": true, "could": true,
	"might": true, "must": true, "ought": true,
	"be": true, "am": true, "is": true, "are": true, "was": true, "were": true,
	"been": true, "being": true, "do": true, "does": true, "did": true, "doing": true,
	"have": true, "has": true, "had": true, "having": true,
	"i": true, "me": true, "my": true, "we": true, "us": true, "our": true,
	"you": true, "your": true, "he": true, "him": true, "his": true, "she": true,
	"her": true, "it": true, "its": true, "they": true, "them": true, "their": true,
	"this": true, "that": true, "these": true, "those": true, "what": true, "which": true,
	"who": true, "whom": true, "whose": true,
	"now": true, "new": true, "old": true, "still": true, "even": true, "ever": true,
	"never": true, "always": true, "already": true, "yet": true, "while": true, "until": true,
	"because": true, "although": true, "though": true, "unless": true, "since": true,
	"whether": true, "either": true, "neither": true, "another": true,

	// 万能动词
	"get": true, "gets": true, "got": true, "getting": true,
	"make": true, "makes": true, "made": true, "making": true,
	"take": true, "takes": true, "took": true, "taking": true,
	"come": true, "comes": true, "came": true, "coming": true,
	"go": true, "goes": true, "went": true, "going": true,
	"see": true, "sees": true, "saw": true, "seen": true,
	"know": true, "knows": true, "known": true,
	"think": true, "thinks": true, "want": true, "wants": true,
	"need": true, "needs": true, "needed": true,
	"use": true, "uses": true, "used": true, "using": true,
	"set": true, "sets": true, "put": true, "puts": true,
	"say": true, "says": true, "said": true,
	"tell": true, "tells": true, "told": true,
	"give": true, "gives": true, "gave": true, "given": true,
	"find": true, "finds": true, "found": true,
	"keep": true, "keeps": true, "kept": true,
	"let": true, "lets": true, "like": true, "likes": true, "liked": true,
	"look": true, "looks": true, "looked": true,
	"seem": true, "seems": true, "seemed": true,
	"become": true, "becomes": true, "became": true,
	"show": true, "shows": true, "showed": true, "shown": true,
	"call": true, "calls": true, "called": true,
	"try": true, "tries": true, "tried": true,
	"start": true, "starts": true, "started": true,
	"end": true, "ends": true, "ended": true,
	"help": true, "helps": true, "helped": true,
	"run": true, "runs": true, "ran": true, "running": true,
	"move": true, "moves": true, "moved": true,
	"turn": true, "turns": true, "turned": true,
	"bring": true, "brings": true, "brought": true,
	"hold": true, "holds": true, "held": true, "holding": true,
	"write": true, "writes": true, "wrote": true, "written": true,
	"provide": true, "provides": true, "provided": true, "providing": true,
	"include": true, "includes": true, "included": true,
	"follow": true, "follows": true, "followed": true,
	"continue": true, "continues": true, "continued": true,
	"announce": true, "announces": true, "announced": true,
	"report": true, "reports": true, "reported": true, "unreported": true,
	"return": true, "returns": true, "returned": true,
	"reach": true, "reaches": true, "reached": true,
	"pass": true, "passes": true, "passed": true,
	"open": true, "opens": true, "opened": true,
	"close": true, "closes": true, "closed": true,
	"file": true, "files": true, "filed": true,
	"form": true, "forms": true, "formed": true,
	"light": true, "lights": true,
	"manage": true, "manages": true, "managed": true,
	"confirm": true, "confirms": true, "confirmed": true,
	"spin": true, "spins": true, "spun": true,
	"dive": true, "dives": true,
	"threaten": true, "threatens": true, "threatened": true,
	"pull": true, "pulled": true,
	"apply": true, "applying": true, "applied": true,

	// 万能名词 / 量词
	"way": true, "ways": true, "thing": true, "things": true,
	"time": true, "times": true, "day": true, "days": true,
	"night": true, "nights": true, "week": true, "weeks": true,
	"month": true, "months": true, "year": true, "years": true,
	"today": true, "tonight": true, "daily": true, "weekly": true,
	"part": true, "parts": true, "place": true, "places": true,
	"case": true, "cases": true, "point": true, "points": true,
	"area": true, "areas": true, "group": true, "groups": true,
	"company": true, "companies": true, "member": true, "members": true,
	"people": true, "person": true, "world": true, "worlds": true,
	"hand": true, "hands": true, "side": true, "sides": true,
	"home": true, "homes": true, "work": true, "works": true, "working": true,
	"number": true, "numbers": true, "level": true, "levels": true,
	"figure": true, "figures": true, "record": true, "records": true,
	"list": true, "lists": true, "link": true, "links": true,
	"back": true, "ahead": true, "top": true, "high": true, "low": true,
	"big": true, "small": true, "long": true, "short": true, "full": true,
	"first": true, "second": true, "third": true, "last": true, "next": true,
	"one": true, "two": true, "three": true, "four": true, "five": true,
	"six": true, "seven": true, "eight": true, "nine": true, "ten": true,
	"hundred": true, "hundreds": true, "thousand": true, "thousands": true,
	"dozen": true, "dozens": true,
	"bit": true, "lot": true, "lots": true, "much": true, "many": true,
	"every": true, "everything": true, "something": true, "anything": true,

	// 时间
	"january": true, "february": true, "march": true, "april": true,
	"june": true, "july": true, "august": true,
	"september": true, "october": true, "november": true, "december": true,
	"spring": true, "summer": true, "autumn": true, "fall": true, "winter": true,
	"monday": true, "tuesday": true, "wednesday": true, "thursday": true,
	"friday": true, "saturday": true, "sunday": true,
	"morning": true, "afternoon": true, "evening": true,
	// 注：may 既是情态又是月份，一律停用

	"american": true, "america": true, "australia": true, "global": true,
	"final": true, "early": true, "late": true, "latest": true, "special": true,
	"fast": true, "slow": true, "soon": true, "later": true,
	"good": true, "best": true, "better": true, "great": true, "major": true,
	"true": true, "false": true, "yes": true,
	"may": true,

	// 域名媒体套话
	"domain": true, "domains": true, "name": true, "names": true,
	"sale": true, "sales": true, "sold": true, "sell": true, "selling": true,
	"buyer": true, "buyers": true, "seller": true, "sellers": true,
	"auction": true, "auctions": true, "market": true, "marketplace": true,
	"aftermarket": true, "registrar": true, "registry": true,
	"tld": true, "tlds": true, "gtld": true, "gtlds": true, "cctld": true, "cctlds": true,
	"com": true, "net": true, "org": true, "info": true, "xyz": true,
	"news": true, "article": true, "articles": true, "update": true, "updates": true,
	"story": true, "stories": true, "post": true, "posts": true,
	"thread": true, "threads": true, "forum": true, "comment": true, "comments": true,
	"discussion": true, "blog": true, "podcast": true,
	"industry": true, "business": true, "opportunity": true, "opportunities": true,
	"success": true, "focus": true, "approach": true, "version": true, "versions": true,
	"event": true, "events": true, "ticket": true, "tickets": true,
	"price": true, "prices": true, "cash": true, "money": true, "dollar": true, "dollars": true,
	"million": true, "billion": true,
	"window": true, "windows": true, "seats": true, "track": true, "chart": true, "charts": true,
	"pioneer": true, "favorite": true, "arena": true, "pack": true, "pool": true, "pools": true,
	"rest": true, "key": true, "corner": true, "family": true,
	"holiday": true, "break": true, "vacation": true, "vacationing": true,
	"content": true, "copyrighted": true, "permission": true, "published": true,
	"website": true, "message": true, "contact": true, "editor": true, "available": true,
	"personal": true, "copyright": true, "rights": true, "reserved": true,
	"http": true, "https": true, "www": true, "html": true, "php": true, "rss": true, "feed": true,
	"click": true, "read": true, "via": true, "per": true, "amp": true,
	"namepros": true, "dnjournal": true, "domainnamewire": true, "dnwire": true,
	"godaddy": true, "namejet": true, "sedo": true, "afternic": true,
	"mark": true, "john": true, "dan": true, "colin": true,
	"completely": true, "previously": true, "sorely": true, "confident": true,
	"easier": true, "easiest": true, "harder": true,
	"round":       true,
	"application": true, "applications": true, "applicant": true, "applicants": true,
	"registration": true, "registrations": true, "registered": true,
	"partnership": true, "partner": true, "partners": true,
	"service": true, "services": true, "backend": true,
	"community": true, "position": true, "board": true, "dominant": true,
	"milestone": true, "anniversary": true, "category": true, "potential": true,
	"unlock": true, "unlocks": true, "control": true, "reaching": true,
	"converting": true, "upcoming": true, "discounted": true, "expires": true,
	"preliminary": true, "agenda": true, "focused": true,
	"brothers": true, "arms": true, "decision": true, "singular": true,
	"game": true, "games": true, "fight": true, "lawsuit": true,
	"bonanza": true, "user": true, "users": true,
	"floodgates": true, "backlog": true, "torrid": true, "eased": true,
	"slowdown": true, "bewildering": true, "decapitates": true, "administrator": true,
	"heavenly": true, "beloved": true, "cost": true,
	"rebuilt": true, "catching": true, "drop": true, "drops": true,
	"liquid": true, "sights": true, "winning": true, "fans": true, "party": true, "hats": true,
	"celebrated": true, "wilderness": true, "alaskan": true, "unreleased": true,
	"competitive": true, "completive": true, "swim": true,
	"submissions": true, "submission": true,
	"bird": true, "hip": true, "hop": true, "dot": true,
	"easy":  true,
	"offer": true, "offers": true, "sld": true, "slds": true,
}

// MergeStopwords 合并内置停用词与外部集合。
func MergeStopwords(extra map[string]bool) map[string]bool {
	out := make(map[string]bool, len(builtinStopwords)+len(extra))
	for w := range builtinStopwords {
		out[w] = true
	}
	for w, ok := range extra {
		if ok && w != "" {
			out[w] = true
		}
	}
	return out
}

// isBrandLike 判断是否更像可投资的品牌/SLD 形态词（用于排序加权）。
func isBrandLike(w string) bool {
	n := len(w)
	if n == 2 {
		return w == "ai" || w == "io" || w == "am"
	}
	if n < 3 || n > 12 {
		return false
	}
	hasDigit := false
	for _, r := range w {
		if r >= '0' && r <= '9' {
			hasDigit = true
			break
		}
	}
	if hasDigit {
		return n <= 8
	}
	if n <= 8 {
		return true
	}
	return !hasCommonEnglishSuffix(w)
}

func hasCommonEnglishSuffix(w string) bool {
	suffixes := []string{"tion", "sion", "ment", "ness", "ance", "ence", "able", "ible"}
	for _, s := range suffixes {
		if len(w) >= 8 && len(w) > len(s)+3 && stringsHasSuffix(w, s) {
			return true
		}
	}
	return false
}

func stringsHasSuffix(s, suffix string) bool {
	return len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix
}
