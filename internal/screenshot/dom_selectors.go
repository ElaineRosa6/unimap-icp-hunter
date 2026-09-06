package screenshot

// engineSelectors defines CSS selectors for extracting structured asset data
// from a search engine's web results page.
type engineSelectors struct {
	RowSelector    string // CSS selector for a single result row
	ExtractJS      string // JavaScript snippet to run on the page for data extraction
	PaginationNext string // CSS selector for the "next page" button
	TotalSelector  string // CSS selector for the total results count indicator
}

// selectorsByEngine maps engine names to their DOM extraction selectors.
// These are best-effort based on current page structures and may need
// adjustment when engine frontends change.
var selectorsByEngine = map[string]*engineSelectors{
	"fofa": {
		RowSelector:    ".hsxa-meta-data-item",
		ExtractJS:      extractFofaJS,
		PaginationNext: "button.btn-next:not([disabled])",
		TotalSelector:  "[class*='total']",
	},
	"hunter": {
		RowSelector:    ".q-table tbody tr",
		ExtractJS:      extractHunterJS,
		PaginationNext: ".q-pagination button:last-child:not([disabled])",
		TotalSelector:  ".page-list-body_statistic",
	},
	"zoomeye": {
		RowSelector:    "div.search-result-item-container",
		ExtractJS:      extractZoomEyeJS,
		PaginationNext: "li.ant-pagination-next:not(.ant-pagination-disabled) a",
		TotalSelector:  "li.ant-pagination-total-text span",
	},
	"quake": {
		RowSelector:    ".item-container",
		ExtractJS:      extractQuakeJS,
		PaginationNext: ".el-pagination__next:not([disabled]) button",
		TotalSelector:  ".total-count",
	},
	"shodan": {
		RowSelector:    ".l-search-results .result",
		ExtractJS:      extractShodanJS,
		PaginationNext: ".pagination .next:not(.disabled) a, a[rel='next'], nav ul li:last-child a",
		TotalSelector:  "h4.total-results, .result-count, [class*='total-results']",
	},
	"censys": {
		RowSelector:    "a[href*='/hosts/']",
		PaginationNext: "[class*='next']",
		ExtractJS:      extractCensysJS,
		TotalSelector:  "[class*='total']",
	},
	"daydaymap": {
		RowSelector:    "tr.ant-table-row",
		PaginationNext: ".el-pagination__next",
		ExtractJS:      extractDayDayMapJS,
		TotalSelector:  "[class*='total']",
	},
}

func getSelectors(engine string) *engineSelectors {
	s, ok := selectorsByEngine[engine]
	if !ok {
		return nil
	}
	return s
}

// JavaScript snippets to extract structured data from each engine's result page.
// Each script returns a JSON string: {"assets":[{ip,port,protocol,...}],"total":N,"hasMore":bool}

const extractFofaJS = `
(function() {
  // FOFA result page (2026-08 hardened: multiple row selector fallbacks).
  var rowSelectors = [
    '.hsxa-meta-data-item',
    '[class*="result-item"]',
    '[class*="search-result"]',
    '.el-table__row'
  ];
  var rows = [];
  var rowSelectorUsed = '';
  for (var si = 0; si < rowSelectors.length; si++) {
    try {
      var nodes = document.querySelectorAll(rowSelectors[si]);
      if (nodes.length > 0) { rows = nodes; rowSelectorUsed = rowSelectors[si]; break; }
    } catch(e) { continue; }
  }

  var assets = [];
  for (var i = 0; i < rows.length; i++) {
    var row = rows[i];
    var asset = {};

    // Live 2026-08-21: span.hsxa-host is IP only; port lives in span.hsxa-port.
    // Older pages still put "IP:port" in .hsxa-host.
    var ipEl = row.querySelector('.hsxa-host, .hsxa-ip a');
    if (ipEl) {
      var ipText = ipEl.textContent.trim();
      var hostPort = ipText.match(/^(\d{1,3}(?:\.\d{1,3}){3})(?::(\d{1,5}))?$/);
      if (hostPort) {
        asset.ip = hostPort[1];
        if (hostPort[2]) asset.port = parseInt(hostPort[2], 10) || 0;
      } else {
        asset.ip = ipText.split(':')[0] || '';
      }
    }
    if (!asset.port) {
      var portEl = row.querySelector('.hsxa-port');
      if (portEl) {
        var portMatch = portEl.textContent.trim().match(/(\d{1,5})/);
        if (portMatch) asset.port = parseInt(portMatch[1], 10) || 0;
      }
    }
    if (!asset.ip) {
      var textNodes = row.querySelectorAll('a, span, div, td');
      for (var tn = 0; tn < textNodes.length; tn++) {
        var t = textNodes[tn].textContent.trim();
        var ipMatch = t.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})(?::(\d+))?$/);
        if (ipMatch) {
          asset.ip = ipMatch[1];
          if (ipMatch[2]) asset.port = parseInt(ipMatch[2]) || 0;
          break;
        }
      }
    }

    // Port from qbase64 links (base64 of "port=")
    if (!asset.port) {
      var portLink = row.querySelector("a[href*='qbase64=cG9ydD0']");
      if (portLink) {
        var pm = portLink.textContent.trim().match(/(\d+)/);
        if (pm) asset.port = parseInt(pm[1]);
      }
    }

    // Protocol from qbase64 links
    var protoLinks = row.querySelectorAll("a[href*='qbase64=']");
    for (var pl = 0; pl < protoLinks.length; pl++) {
      var ptext = protoLinks[pl].textContent.trim().toLowerCase();
      if (ptext === 'http' || ptext === 'https' || ptext === 'tcp' || ptext === 'udp') {
        asset.protocol = ptext; break;
      }
    }

    // Host, title, country, org from label-value field pairs
    var fields = row.querySelectorAll('.hsxa-meta-data-item__field, [class*="field"], [class*="item"]');
    for (var fi = 0; fi < fields.length; fi++) {
      var f = fields[fi];
      var labelEl = f.querySelector('[class*="label"], [class*="key"], dt, th');
      var valueEl = f.querySelector('[class*="value"], dd, td');
      var label = labelEl ? labelEl.textContent.trim() : '';
      var value = valueEl ? valueEl.textContent.trim() : f.textContent.trim();
      if (!value || value === label) continue;
      var ll = label.toLowerCase();
      if ((ll.indexOf('域名') >= 0 || ll.indexOf('host') >= 0 || ll.indexOf('domain') >= 0) && !asset.host && value !== asset.ip) asset.host = value;
      if ((ll.indexOf('标题') >= 0 || ll.indexOf('title') >= 0) && !asset.title) asset.title = value;
      if ((ll.indexOf('国家') >= 0 || ll.indexOf('country') >= 0) && !asset.country) asset.country = value;
      if ((ll.indexOf('组织') >= 0 || ll.indexOf('org') >= 0 || ll.indexOf('isp') >= 0) && !asset.org) asset.org = value;
      if ((ll.indexOf('server') >= 0 || ll.indexOf('产品') >= 0 || ll.indexOf('product') >= 0) && !asset.server) asset.server = value;
      if ((ll.indexOf('端口') >= 0 || ll.indexOf('port') >= 0) && !asset.port) { var pp = parseInt(value); if (pp > 0) asset.port = pp; }
    }

    // Title fallback
    if (!asset.title) {
      var titleEl = row.querySelector('[class*="title"], .hsxa-title, a[title]');
      if (titleEl) asset.title = (titleEl.getAttribute('title') || titleEl.textContent).trim();
    }

    asset.source = 'fofa';
    if (asset.ip || asset.host) assets.push(asset);
  }

  // Total count
  var total = 0;
  var totalSelectors = ['.hsxa-result-total', '[class*="result"] [class*="total"]', '[class*="total"]'];
  for (var ts = 0; ts < totalSelectors.length; ts++) {
    var totalEl = document.querySelector(totalSelectors[ts]);
    if (totalEl) {
      var tm = totalEl.textContent.match(/(\d[\d,]*)/);
      if (tm) { total = parseInt(tm[0].replace(/,/g, '')); break; }
    }
  }

  // Pagination
  var hasNext = false;
  try { hasNext = !!document.querySelector('button.btn-next:not([disabled])'); } catch(e) {}
  if (!hasNext) {
    try { hasNext = !!document.querySelector('.el-pagination .btn-next:not([disabled])'); } catch(e) {}
  }
  if (!hasNext) {
    var nextBtn = document.querySelector('[class*="next"]');
    hasNext = !!nextBtn && !nextBtn.hasAttribute('disabled') && nextBtn.className.indexOf('disabled') < 0;
  }

  return JSON.stringify({
    assets: assets, total: total, hasMore: hasNext,
    rowSelectorUsed: rowSelectorUsed, rowsFound: rows.length,
    extractionError: rows.length === 0 ? 'no_result_rows' : (assets.length === 0 ? 'rows_contained_no_assets' : '')
  });
})()
`

const extractHunterJS = `
(function() {
  var rows = document.querySelectorAll('.q-table tbody tr');
  var assets = [];
  var seen = {};
  rows.forEach(function(row) {
    var cells = row.querySelectorAll('td');
    if (cells.length < 5) return;
    var asset = {};

    // Hunter Quasar UI columns (2026-07-29 layout, no checkbox column):
    // 0=序号, 1=IP, 2=域名, 3=端口/服务, 4=标题, 5=状态码, 6=ICP, 7=应用/组件, 8=标签, 9=地区, 10=更新时间, 11=操作
    // Auto-detect checkbox column: if cells[0] has a checkbox, shift indices by +1.
    var offset = 0;
    if (cells[0] && cells[0].querySelector('input[type="checkbox"], .q-checkbox')) {
      offset = 1;
    }

    function getCellText(idx) {
      idx += offset;
      if (idx >= cells.length) return '';
      var cell = cells[idx];
      var cellDiv = cell.querySelector('.cell');
      var text = cellDiv ? cellDiv.textContent : cell.textContent;
      text = text.replace(/只看该[^\s]*不看该[^\s]*/g, '');
      text = text.replace(/只看空[^\s]*不看空[^\s]*/g, '');
      text = text.replace(/看相似(网站|icon)/g, '');
      text = text.replace(/访问[^\s]*/g, '');
      text = text.replace(/复制[^\s]*/g, '');
      text = text.replace(/云厂商/g, '');
      text = text.replace(/共\d+条/g, '');
      text = text.replace(/共\d+个/g, '');
      return text.replace(/\s+/g, ' ').trim();
    }

    asset.ip = getCellText(1);
    var hostText = getCellText(2);
    if (hostText && hostText !== asset.ip) asset.host = hostText;
    // Port/protocol from column 3 (format: "80 http")
    var portText = getCellText(3);
    var pm = portText.match(/(\d{1,5})/);
    if (pm) asset.port = parseInt(pm[1]);
    var protoMatch = portText.match(/\b(http|https|tls|ssl|tcp|udp|ssh|ftp|smtp|pop3|imap|mysql|rdp|smb|dns)\b/i);
    if (protoMatch) asset.protocol = protoMatch[1].toLowerCase();
    // Title from column 4
    var titleRaw = getCellText(4);
    if (titleRaw) {
      var titleParts = titleRaw.split(/\s+(?:企业|个人|开源|政府|金融)/);
      asset.title = titleParts[0].trim();
    }
    // Status code from column 5
    var statusText = getCellText(5);
    var sm = statusText.match(/(\d{3})/);
    if (sm) asset.status_code = parseInt(sm[1]);
    // ICP from column 6
    var icpText = getCellText(6);
    if (icpText) asset.icp = icpText;
    // App/component from column 7 → product (falls back to title later)
    var productText = getCellText(7);
    if (productText) asset.product = productText;
    // Tags from column 8 → extra.tags
    var tagsText = getCellText(8);
    if (tagsText) asset.tags = tagsText;
    // Update time from column 10 → last_seen
    var lastSeenText = getCellText(10);
    if (lastSeenText) asset.last_seen = lastSeenText;
    // Region from column 9
    var regionText = getCellText(9);
    if (regionText) asset.region = regionText;
    asset.source = 'hunter';

    if (!asset.ip && !asset.host) return;
    if (asset.ip && !/^\d{1,3}(?:\.\d{1,3}){3}$/.test(asset.ip) && asset.ip.indexOf(':') < 0) return;
    var key = asset.ip + ':' + asset.port;
    if (asset.port > 0 && seen[key]) return;
    if (asset.port > 0) seen[key] = true;
    assets.push(asset);
  });
  var totalEl = document.querySelector('.statistic, .page-list-body_statistic');
  var total = 0;
  if (totalEl) {
    var m = totalEl.textContent.match(/资产总数[\s\S]*?(\d[\d,]*)/);
    if (m) total = parseInt(m[1].replace(/,/g, ''));
    if (!total) { var m2 = totalEl.textContent.match(/(\d[\d,]*)/); if (m2) total = parseInt(m2[1].replace(/,/g, '')); }
  }
  var hasNext = !!document.querySelector('.q-pagination button:last-child:not([disabled]), .q-table__pagination button:last-child:not([disabled])');
  return JSON.stringify({assets: assets, total: total, hasMore: hasNext});
})()
`

const extractZoomEyeJS = `
(function() {
  var containers = document.querySelectorAll('div.search-result-item-container');
  var assets = [];
  containers.forEach(function(container) {
    var asset = {};
    // Extract IP: prefer url-container text (stable), fallback to ip-detail-box span
    var ipEl = container.querySelector('div.url-container span, div.ip-detail-box span, div.header-bar span');
    if (ipEl) {
      var ipText = ipEl.textContent.trim();
      var ipMatch = ipText.match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
      if (ipMatch) {
        asset.ip = ipMatch[1];
      } else if (!asset.host) {
        asset.host = ipText;
      }
    }
    if (asset.host) {
      var hostPort = asset.host.match(/^([A-Za-z0-9.-]+\.[A-Za-z]{2,}):(\d{1,5})$/);
      if (hostPort) {
        asset.host = hostPort[1];
        if (!asset.port) asset.port = parseInt(hostPort[2], 10) || 0;
      }
    }
    if (asset.ip && /[A-Za-z]/.test(asset.ip)) {
      if (!asset.host) asset.host = asset.ip;
      asset.ip = '';
    }
    // Extract host:port from header-bar > div.url-container
    var urlContainer = container.querySelector('div.url-container');
    if (urlContainer) {
      var urlText = urlContainer.textContent.trim();
      // Parse "domain:port" or "ip:port" format
      var match = urlText.match(/^(.+?):(\d+)$/);
      if (match) {
        if (!asset.ip || /\d+\.\d+\.\d+\.\d+/.test(match[1])) {
          asset.ip = match[1];
        }
        asset.host = /\d+\.\d+\.\d+\.\d+/.test(match[1]) ? '' : match[1];
        asset.port = parseInt(match[2]) || 0;
      } else {
        asset.host = urlText;
      }
    }
    // Extract port from protocol-port-box first button
    var portBtn = container.querySelector('div.protocol-port-box button:first-child');
    if (portBtn) {
      var portVal = parseInt(portBtn.textContent.trim());
      if (portVal > 0) asset.port = portVal;
    }
    // Extract protocol from protocol-port-box last button span
    var protoBtn = container.querySelector('div.protocol-port-box button:last-child span');
    if (protoBtn) {
      asset.protocol = protoBtn.textContent.trim();
    }
    // Extract banner from pre tab panel
    var preEl = container.querySelector('pre');
    if (preEl) {
      asset.banner = preEl.textContent.trim().substring(0, 500);
    }
    // Extract title/org/asn/host/isp/country/timestamp from labelled
    // router-container blocks inside search-result-item-info.
    // ZoomEye cards store metadata as <span>label:</span> + url-container value,
    // NOT as concatenated text. The old [class*='title'] selector matched
    // unrelated elements (.search-result-item-tabs etc.), so we use precise traversal.
    var infoEl = container.querySelector('div.search-result-item-info');
    if (infoEl) {
      var rcNodes = infoEl.querySelectorAll('div.router-container');
      for (var ri = 0; ri < rcNodes.length; ri++) {
        var rcLabel = rcNodes[ri].querySelector('span.whitespace-nowrap');
        var rcValue = rcNodes[ri].querySelector('div.url-container span');
        if (!rcLabel || !rcValue) continue;
        var lt = rcLabel.textContent.trim();
        var lv = rcValue.textContent.trim();
        if (!lv) continue;
        if (lt.indexOf('标题:') === 0) { if (!asset.title) asset.title = lv; }
        else if (lt.indexOf('组织:') === 0) { if (!asset.org) asset.org = lv; }
        else if (lt.indexOf('ASN:') === 0) { if (!asset.asn) asset.asn = lv; }
        else if (lt.indexOf('主机名:') === 0) { if (!asset.host) asset.host = lv; }
        else if (lt.indexOf('ISP:') === 0) { if (!asset.isp) asset.isp = lv; }
      }
      // Country: extract from flag-XX class (e.g. flag-cn → CN)
      if (!asset.country_code) {
        var flagEl = infoEl.querySelector('span.flag');
        if (flagEl) {
          var fm = (flagEl.className || '').match(/flag-([a-z]{2})/i);
          if (fm) asset.country_code = fm[1].toUpperCase();
        }
      }
      // Timestamp: search-result-icon-time paragraph
      if (!asset.last_seen) {
        var timeEl = infoEl.querySelector('p.search-result-icon-time');
        if (timeEl) asset.last_seen = timeEl.textContent.trim();
      }
    }
    asset.source = 'zoomeye';
    if (asset.ip || asset.host) {
      assets.push(asset);
    }
  });
  // Get total from pagination
  var totalEl = document.querySelector('li.ant-pagination-total-text span');
  var total = 0;
  if (totalEl) {
    var m = totalEl.textContent.match(/[\d,]+/);
    if (m) total = parseInt(m[0].replace(/,/g, '')) || 0;
  }
  var hasNext = !!document.querySelector('li.ant-pagination-next:not(.ant-pagination-disabled) a');
  return JSON.stringify({assets: assets, total: total, hasMore: hasNext});
})()
`

const extractQuakeJS = `
(function() {
  var containers = document.querySelectorAll('.item-container');
  var assets = [];
  containers.forEach(function(container) {
    var asset = {};
    // IP from div.ip span.copy_btn data-clipboard-text
    var copyBtn = container.querySelector('div.ip span.copy_btn, [data-clipboard-text]');
    if (copyBtn) {
      var clipText = copyBtn.getAttribute('data-clipboard-text') || '';
      var v4 = clipText.match(/^(\d{1,3}(?:\.\d{1,3}){3})(?::(\d{1,5}))?$/);
      if (v4) {
        asset.ip = v4[1];
        if (v4[2]) asset.port = parseInt(v4[2], 10) || 0;
      }
    }
    // Port from span.port
    var portEl = container.querySelector('span.port');
    if (portEl) {
      var p = parseInt(portEl.textContent.trim());
      if (p > 0) asset.port = p;
    }
    // Protocol from span.server-protocol
    var protoEl = container.querySelector('span.server-protocol');
    if (protoEl) asset.protocol = protoEl.textContent.trim();
    // Title from .title-line span.ellipse-text
    var titleEl = container.querySelector('.title-line span.ellipse-text');
    if (titleEl) asset.title = titleEl.textContent.trim();
    // Country from .country-container .address
    var countryEl = container.querySelector('.country-container .address');
    if (countryEl) asset.country = countryEl.textContent.trim();
    // Host from .item span.label matching "host" + sibling .ellipse-text
    var items = container.querySelectorAll('.item');
    items.forEach(function(item) {
      var label = item.querySelector('.label');
      if (label && /host|domain/i.test(label.textContent)) {
        var val = item.querySelector('.ellipse-text');
        if (val) {
          var hostVal = val.textContent.trim();
          if (hostVal && hostVal !== '--' && hostVal !== '-' && hostVal !== asset.ip) asset.host = hostVal;
        }
      }
    });
    if (asset.host === '--' || asset.host === '-') asset.host = '';
    asset.source = 'quake';
    if (asset.ip || asset.host) assets.push(asset);
  });
  var totalEl = document.querySelector('.total-count');
  var total = 0;
  if (totalEl) { var m = totalEl.textContent.match(/(\d[\d,]*)/); if (m) total = parseInt(m[0].replace(/,/g, '')); }
  var hasNext = !!document.querySelector('.el-pagination__next:not([disabled]) button');
  return JSON.stringify({assets: assets, total: total, hasMore: hasNext});
})()
`

const extractShodanJS = `
(function() {
  // 2026-08-21 layout: .l-search-results > div.result (no .row wrapper).
  // Host links use /host/<ipv4|ipv6>; IPv6 service URLs are http://[v6]:port.
  // Do not use /host/ regex literals here: they break when this script lives in a Go raw string.
  function ipFromHostPath(href) {
    if (!href) return '';
    var marker = '/host/';
    var i = href.indexOf(marker);
    if (i < 0) return '';
    var rest = href.slice(i + marker.length);
    var end = -1;
    for (var k = 0; k < rest.length; k++) {
      var ch = rest.charAt(k);
      if (ch === '/' || ch === '?' || ch === '#') { end = k; break; }
    }
    var ip = end >= 0 ? rest.slice(0, end) : rest;
    try { ip = decodeURIComponent(ip); } catch (e) {}
    return ip;
  }
  function portFromHref(href) {
    if (!href) return 0;
    var br = href.lastIndexOf(']:');
    if (br >= 0) return parseInt(href.slice(br + 2), 10) || 0;
    var scheme = href.indexOf('://');
    var start = scheme >= 0 ? scheme + 3 : 0;
    var colon = href.lastIndexOf(':');
    if (colon <= start) return 0;
    return parseInt(href.slice(colon + 1), 10) || 0;
  }

  var rowSelectors = [
    '.l-search-results .result',
    'div.result',
    '.row.l-search-results .result',
    '[class*="search-result"]'
  ];
  var results = [];
  var rowSelectorUsed = '';
  for (var selIdx = 0; selIdx < rowSelectors.length; selIdx++) {
    try {
      var nodes = document.querySelectorAll(rowSelectors[selIdx]);
      if (nodes.length > 0) {
        results = nodes;
        rowSelectorUsed = rowSelectors[selIdx];
        break;
      }
    } catch (e) {}
  }

  var assets = [];
  for (var i = 0; i < results.length; i++) {
    var el = results[i];
    var asset = {};
    var ipLink = el.querySelector("div.heading a.title[href*='/host/'], a.title[href*='/host/'], a[href*='/host/']");
    if (ipLink) {
      asset.ip = ipFromHostPath(ipLink.getAttribute('href') || '');
      asset.title = (ipLink.textContent || '').trim();
    }
    if (!asset.ip) {
      var hostLi = el.querySelector('li.hostnames');
      if (hostLi) asset.ip = (hostLi.textContent || '').trim();
    }
    var portLink = el.querySelector("div.heading a.text-danger, div.heading a[href^='http://'], div.heading a[href^='https://']");
    if (portLink) asset.port = portFromHref(portLink.getAttribute('href') || '');
    var tsEl = el.querySelector('div.heading div.timestamp, .timestamp, time');
    if (tsEl) asset.last_seen = (tsEl.textContent || '').trim();
    var orgLink = el.querySelector('.result-details a.filter-link.filter-org, a.filter-org');
    if (orgLink) asset.org = (orgLink.textContent || '').trim();
    var countryEl = el.querySelector('img.flag + a');
    if (countryEl) asset.country_code = (countryEl.textContent || '').trim();
    var banner = el.querySelector('.banner-data pre, pre');
    if (banner) asset.banner = (banner.textContent || '').trim().substring(0, 200);
    var hostEl = el.querySelector('li.hostnames');
    if (hostEl) {
      var hostText = (hostEl.textContent || '').trim();
      if (hostText && hostText !== asset.ip) asset.host = hostText;
    }
    asset.source = 'shodan';
    if (asset.ip || asset.host) assets.push(asset);
  }

  var total = 0;
  var totalSelectors = ['h4.total-results', '.total-results', '.result-count'];
  for (var t = 0; t < totalSelectors.length; t++) {
    var totalEl = document.querySelector(totalSelectors[t]);
    if (!totalEl) continue;
    var totalMatch = (totalEl.textContent || '').match(/([\d,]+)/);
    if (totalMatch) {
      total = parseInt(totalMatch[1].replace(/,/g, ''), 10) || 0;
      break;
    }
  }

  var hasNext = false;
  try { hasNext = !!document.querySelector('a[rel="next"]'); } catch (e) {}
  try { if (!hasNext && document.querySelector('.pagination .next:not(.disabled) a')) hasNext = true; } catch (e) {}

  return JSON.stringify({
    assets: assets,
    total: total,
    hasMore: hasNext,
    rowSelectorUsed: rowSelectorUsed,
    rowsFound: results.length
  });
})()
`

const extractCensysJS = `
(function() {
  var rowSelectors = [
    "[data-search-results-layout] a[href*='/hosts/']",
    "a[href*='/hosts/']",
    "[class*='result-card']", "[class*='search-result']",
    "[class*='result-list'] > div",
    "table tbody tr", ".host-row", "[data-testid*='result']"
  ];
  var rows = [];
  var rowSelectorUsed = '';
  for (var si = 0; si < rowSelectors.length; si++) {
    try {
      var nodes = document.querySelectorAll(rowSelectors[si]);
      if (nodes.length > 0) { rows = nodes; rowSelectorUsed = rowSelectors[si]; break; }
    } catch(e) { continue; }
  }
  var assets = [];
  for (var i = 0; i < rows.length; i++) {
    var row = rows[i];
    var asset = {};
    var ipLink = row;
    if (!(row.tagName && row.tagName.toLowerCase() === 'a' && (row.getAttribute('href') || '').indexOf('/hosts/') >= 0)) {
      ipLink = row.querySelector("a[href*='/hosts/']");
    }
    if (ipLink) {
      var href = ipLink.getAttribute('href') || '';
      var m = href.match(/\/hosts\/([^/?#]+)/);
      if (m) {
        try { asset.ip = decodeURIComponent(m[1]); } catch (e) { asset.ip = m[1]; }
      }
      if (!asset.title) asset.title = ipLink.textContent.trim();
    }
    if (!asset.ip) {
      var ipEl = row.querySelector("[class*='ip'], [data-ip]");
      if (ipEl) {
        var ipMatch = ipEl.textContent.trim().match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
        if (ipMatch) asset.ip = ipMatch[1];
      }
    }
    if (!asset.ip) {
      var allText = row.querySelectorAll('a, span, div, td');
      for (var tn = 0; tn < allText.length; tn++) {
        var t = allText[tn].textContent.trim();
        var ipM = t.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$/);
        if (ipM) { asset.ip = ipM[1]; break; }
      }
    }
    var portEl = row.querySelector("[class*='port'], [data-port]");
    if (portEl) { var pm = portEl.textContent.trim().match(/(\d+)/); if (pm) asset.port = parseInt(pm[1]) || 0; }
    var protoEl = row.querySelector("[class*='service'], [class*='protocol']");
    if (protoEl) asset.protocol = protoEl.textContent.trim().toLowerCase();
    var hostEl = row.querySelector("[class*='hostname'], [class*='domain']");
    if (hostEl) asset.host = hostEl.textContent.trim();
    var countryEl = row.querySelector("[class*='country'], [class*='location']");
    if (countryEl) asset.country_code = countryEl.textContent.trim();
    var orgEl = row.querySelector("[class*='org'], [class*='organization']");
    if (orgEl) asset.org = orgEl.textContent.trim();
    var osEl = row.querySelector("[class*='os'], [class*='operating']");
    if (osEl) asset.os = osEl.textContent.trim();
    asset.source = 'censys';
    if (asset.ip) assets.push(asset);
  }
  var total = 0;
  var totalSelectors = ["[class*='total']", "[class*='count']", "[data-testid*='total']"];
  for (var t = 0; t < totalSelectors.length; t++) {
    var totalEl = document.querySelector(totalSelectors[t]);
    if (totalEl) { var totalMatch = totalEl.textContent.match(/([\d,]+)/); if (totalMatch) { total = parseInt(totalMatch[1].replace(/,/g, '')); break; } }
  }
  var hasNext = false;
  var nextSelectors = ["[class*='next']", "button[aria-label='next']", "a[rel='next']"];
  for (var n = 0; n < nextSelectors.length; n++) {
    try { var nextEl = document.querySelector(nextSelectors[n]); if (nextEl && !nextEl.disabled) { hasNext = true; break; } } catch(e) { continue; }
  }
  return JSON.stringify({
    assets: assets, total: total, hasMore: hasNext,
    rowSelectorUsed: rowSelectorUsed, rowsFound: rows.length,
    extractionError: rows.length === 0 ? 'no_result_rows' : (assets.length === 0 ? 'rows_contained_no_assets' : '')
  });
})()
`

const extractDayDayMapJS = `
(function() {
  var rowSelectors = [
    "tr.ant-table-row", ".ant-table-row",
    "[class*='result-item']", "[class*='result-card']",
    "[class*='result-list'] > div",
    ".el-table__row", "table tbody tr", ".list_content > div"
  ];
  var rows = [];
  var rowSelectorUsed = '';
  for (var si = 0; si < rowSelectors.length; si++) {
    try {
      var nodes = document.querySelectorAll(rowSelectors[si]);
      if (nodes.length > 0) { rows = nodes; rowSelectorUsed = rowSelectors[si]; break; }
    } catch(e) { continue; }
  }
  var assets = [];
  // Current card view puts the endpoint URL in the card title, not an IP cell.
  // Scope to card headers: arbitrary page links and nested response tables are
  // not asset rows. Preserve distinct ports while deduplicating repeated titles.
  var cardTitles = document.querySelectorAll('.ant-pro-card > .ant-pro-card-header .ant-pro-card-title');
  var seenEndpoints = {};
  for (var ct = 0; ct < cardTitles.length; ct++) {
    var titleNodes = cardTitles[ct].querySelectorAll('span, a');
    for (var cn = 0; cn < titleNodes.length; cn++) {
      var endpointText = titleNodes[cn].textContent.trim();
      if (!/^https?:\/\//i.test(endpointText) || /\s/.test(endpointText)) continue;
      try {
        var endpoint = new URL(endpointText);
        if (endpoint.username || endpoint.password) continue;
        var endpointIP = endpoint.hostname.replace(/^\[|\]$/g, '');
        var ipv4 = /^(\d{1,3}\.){3}\d{1,3}$/.test(endpointIP) && endpointIP.split('.').every(function(part) { return Number(part) <= 255; });
        var ipv6 = endpoint.hostname.charAt(0) === '[' && endpointIP.indexOf(':') >= 0;
        if (!ipv4 && !ipv6) continue;
        var endpointPort = endpoint.port ? Number(endpoint.port) : (endpoint.protocol === 'https:' ? 443 : 80);
        if (endpointPort < 1 || endpointPort > 65535) continue;
        var endpointKey = endpoint.protocol + '|' + endpointIP + '|' + endpointPort;
        if (seenEndpoints[endpointKey]) continue;
        seenEndpoints[endpointKey] = true;
        assets.push({ip: endpointIP, port: endpointPort, protocol: endpoint.protocol.slice(0, -1), source: 'daydaymap'});
      } catch (e) { continue; }
    }
  }
  if (assets.length > 0) {
    rows = cardTitles;
    rowSelectorUsed = '.ant-pro-card > .ant-pro-card-header .ant-pro-card-title';
  }
  var cardView = assets.length > 0;

  for (var i = 0; !cardView && i < rows.length; i++) {
    var row = rows[i];
    if ((row.className || '').indexOf('measure-row') >= 0) continue;
    var asset = {};
    var cells = row.querySelectorAll('td');
    if (cells.length >= 4) {
      function cellText(idx) {
        if (idx >= cells.length) return '';
        return (cells[idx].textContent || '').replace(/\s+/g, ' ').trim();
      }
      var ipCell = cellText(1);
      var ipCellMatch = ipCell.match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
      if (ipCellMatch) asset.ip = ipCellMatch[1];
      var hostCell = cellText(2);
      if (hostCell && hostCell !== asset.ip && hostCell !== '-' && hostCell !== '--') asset.host = hostCell.split(' ')[0];
      var portCell = cellText(3).match(/^(\d{1,5})\b/);
      if (portCell) asset.port = parseInt(portCell[1], 10) || 0;
      var protoCell = cellText(4).match(/\b(https?|tcp|udp|ssh|ftp|smtp|ssl|tls)\b/i);
      if (protoCell) asset.protocol = protoCell[1].toLowerCase();
    }
    var ipLink = row.querySelector("a[href*='ip='], a[href*='/host/']");
    if (ipLink) {
      var ipText = ipLink.textContent.trim();
      var ipMatch = ipText.match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/);
      if (ipMatch) asset.ip = ipMatch[1];
    }
    if (!asset.ip) {
      var ipEl = row.querySelector("[class*='ip'], [data-ip]");
      if (ipEl) { var m = ipEl.textContent.trim().match(/(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})/); if (m) asset.ip = m[1]; }
    }
    if (!asset.ip) {
      var textNodes = row.querySelectorAll('a, span, div, td');
      for (var tn = 0; tn < textNodes.length; tn++) {
        var t = textNodes[tn].textContent.trim();
        var ipM = t.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})(?::(\d+))?$/);
        if (ipM) { asset.ip = ipM[1]; if (ipM[2]) asset.port = parseInt(ipM[2]) || 0; break; }
      }
    }
    if (!asset.port) {
      var portEl = row.querySelector("[class*='port'], [data-port]");
      if (portEl) { var pm = portEl.textContent.trim().match(/(\d+)/); if (pm) asset.port = parseInt(pm[1]) || 0; }
    }
    var protoEl = row.querySelector("[class*='protocol'], [class*='service']");
    if (protoEl) asset.protocol = protoEl.textContent.trim().toLowerCase();
    var hostEl = row.querySelector("[class*='domain'], [class*='host'], a[href*='domain=']");
    if (hostEl) asset.host = hostEl.textContent.trim();
    var titleEl = row.querySelector("[class*='title'], [class*='name']");
    if (titleEl) asset.title = titleEl.textContent.trim();
    var countryEl = row.querySelector("[class*='country'], [class*='location']");
    if (countryEl) asset.country_code = countryEl.textContent.trim();
    var orgEl = row.querySelector("[class*='org'], [class*='company']");
    if (orgEl) asset.org = orgEl.textContent.trim();
    var serverEl = row.querySelector("[class*='server']");
    if (serverEl) asset.server = serverEl.textContent.trim();
    asset.source = 'daydaymap';
    if (asset.ip) assets.push(asset);
  }
  // The current DayDayMap result grid is virtualized and its generated row
  // classes are not stable. If row-oriented selectors miss it, collect exact
  // IPv4 leaf nodes from the rendered grid. This fallback intentionally runs
  // only after the structured row path produced no assets.
  if (assets.length === 0) {
    var seenIPs = {};
    var candidates = document.querySelectorAll('a, span, div, td');
    for (var ci = 0; ci < candidates.length; ci++) {
      var candidate = candidates[ci];
      var candidateText = candidate.textContent.trim();
      var candidateIP = candidateText.match(/^(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})$/);
      if (!candidateIP || seenIPs[candidateIP[1]]) continue;
      seenIPs[candidateIP[1]] = true;

      var container = candidate.closest('tr, [role="row"], [class*="table-row"], [class*="list-row"], [class*="result-row"]');
      if (!container) {
        container = candidate;
        for (var up = 0; up < 4 && container.parentElement; up++) container = container.parentElement;
      }
      var containerText = container.textContent.replace(/\s+/g, ' ').trim();
      var asset = {ip: candidateIP[1], source: 'daydaymap'};
      var protocolMatch = containerText.match(/\b(https?|tcp|udp|ssh|smtp|ftp)\b/i);
      if (protocolMatch) asset.protocol = protocolMatch[1].toLowerCase();
      var explicitPort = container.querySelector("[class*='port'], [data-port]");
      if (explicitPort) {
        var portMatch = explicitPort.textContent.trim().match(/^(\d{1,5})$/);
        if (portMatch) {
          var parsedPort = parseInt(portMatch[1]);
          if (parsedPort > 0 && parsedPort <= 65535) asset.port = parsedPort;
        }
      }
      assets.push(asset);
    }
    if (assets.length > 0) rowSelectorUsed = 'virtual-table-ip-leaf-fallback';
  }
  var total = 0;
  var totalSelectors = ["[class*='total']", "[class*='count']", ".el-pagination__total"];
  for (var t = 0; t < totalSelectors.length; t++) {
    var totalEl = document.querySelector(totalSelectors[t]);
    if (totalEl) { var totalMatch = totalEl.textContent.match(/([\d,]+)/); if (totalMatch) { total = parseInt(totalMatch[1].replace(/,/g, '')); break; } }
  }
  var hasNext = false;
  var nextSelectors = [".el-pagination__next:not([disabled])", "[class*='next']", "button.btn-next:not([disabled])"];
  for (var n = 0; n < nextSelectors.length; n++) {
    try { if (document.querySelector(nextSelectors[n])) { hasNext = true; break; } } catch(e) { continue; }
  }
  return JSON.stringify({
    assets: assets, total: total, hasMore: hasNext,
    rowSelectorUsed: rowSelectorUsed, rowsFound: rows.length,
    extractionError: rows.length === 0 ? 'no_result_rows' : (assets.length === 0 ? 'rows_contained_no_assets' : '')
  });
})()
`
