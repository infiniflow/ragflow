import logging
from abc import ABC
from serpapi import GoogleSearch as SerpApiSearch
import pandas as pd
from agent.component.base import ComponentBase, ComponentParamBase
import requests
import random
from time import sleep
import re
from bs4 import BeautifulSoup
from requests import get
from urllib.parse import unquote, urlparse
def get_useragent():
    """
    Generates a random user agent string mimicking the format of various software versions.

    The user agent string is composed of:
    - Lynx version: Lynx/x.y.z where x is 2-3, y is 8-9, and z is 0-2
    - libwww version: libwww-FM/x.y where x is 2-3 and y is 13-15
    - SSL-MM version: SSL-MM/x.y where x is 1-2 and y is 3-5
    - OpenSSL version: OpenSSL/x.y.z where x is 1-3, y is 0-4, and z is 0-9

    Returns:
        str: A randomly generated user agent string.
    """
    lynx_version = f"Lynx/{random.randint(2, 3)}.{random.randint(8, 9)}.{random.randint(0, 2)}"
    libwww_version = f"libwww-FM/{random.randint(2, 3)}.{random.randint(13, 15)}"
    ssl_mm_version = f"SSL-MM/{random.randint(1, 2)}.{random.randint(3, 5)}"
    openssl_version = f"OpenSSL/{random.randint(1, 3)}.{random.randint(0, 4)}.{random.randint(0, 9)}"
    return f"{lynx_version} {libwww_version} {ssl_mm_version} {openssl_version}"

def _req(term, results, lang, start, proxies, timeout, safe, ssl_verify, region):
    resp = get(
        url="https://www.google.com/search",
        headers={
            "User-Agent": get_useragent(),
            "Accept": "*/*"
        },
        params={
            "q": term,
            "num": results + 2,  # Prevents multiple requests
            "hl": lang,
            "start": start,
            "safe": safe,
            "gl": region,
        },
        proxies=proxies,
        timeout=timeout,
        verify=ssl_verify,
        cookies = {
            'CONSENT': 'PENDING+987', # Bypasses the consent page
            'SOCS': 'CAESHAgBEhIaAB',
        }
    )
    resp.raise_for_status()
    return resp

def fetch_page_content(url, timeout=5):
    """
    Fetches and extracts the text content from a webpage.
    
    Args:
        url: URL to fetch
        timeout: Request timeout in seconds
    
    Returns:
        dict: Contains page title, main content, metadata, and structured content
    """
    try:
        resp = get(
            url=url,
            headers={"User-Agent": get_useragent()},
            timeout=timeout
        )
        resp.raise_for_status()
        
        # Parse the HTML content
        soup = BeautifulSoup(resp.text, "html.parser")
        
        # Remove script and style elements that might contain non-content text
        for script in soup(["script", "style", "noscript", "iframe", "nav"]):
            script.extract()
        
        # Get page title
        page_title = soup.title.text.strip() if soup.title else ""
        
        # Get meta description
        meta_desc = ""
        meta_desc_tag = soup.find("meta", attrs={"name": "description"})
        if meta_desc_tag and "content" in meta_desc_tag.attrs:
            meta_desc = meta_desc_tag["content"]
        
        # Get main content - prioritize main content areas
        main_content = ""
        content_tags = soup.find_all(["article", "main", "div", "section"], 
                                    class_=re.compile(r"content|article|post|entry|text|body", re.I))
        if content_tags:
            main_content = "\n\n".join([tag.get_text(strip=True, separator=" ") for tag in content_tags])
        
        # If no content found, get all paragraph text
        if not main_content:
            paragraphs = soup.find_all("p")
            main_content = "\n".join([p.get_text(strip=True) for p in paragraphs if len(p.get_text(strip=True)) > 50])
        
        # Get headings for structure
        headings = []
        for h in soup.find_all(["h1", "h2", "h3"]):
            headings.append({"level": int(h.name[1]), "text": h.get_text(strip=True)})
        
        # Extract lists
        lists = []
        for list_tag in soup.find_all(["ul", "ol"]):
            list_items = [li.get_text(strip=True) for li in list_tag.find_all("li")]
            lists.append(list_items)
            
        # Get full text content
        full_text = soup.get_text(separator=" ", strip=True)
        
        # Get structured data if available
        structured_data = {}
        for script in soup.find_all("script", attrs={"type": "application/ld+json"}):
            try:
                import json
                data = json.loads(script.string)
                structured_data = data
                break
            except:
                pass
                
        return {
            "title": page_title,
            "meta_description": meta_desc,
            "main_content": main_content,
            "full_text": full_text,
            "headings": headings,
            "lists": lists,
            "structured_data": structured_data,
            "html_lang": soup.html.get("lang", "") if soup.html else "",
            "word_count": len(full_text.split())
        }
        
    except Exception as e:
        return {
            "error": str(e),
            "title": "",
            "meta_description": "",
            "main_content": "",
            "full_text": "",
            "headings": [],
            "lists": [],
            "structured_data": {},
            "html_lang": "",
            "word_count": 0
        }

class SearchResult:
    def __init__(self, url, title, description, source=None, date=None, result_type=None, 
                 image_url=None, related_links=None, page_content=None, snippet_blocks=None):
        self.url = url
        self.title = title
        self.description = description
        self.source = source  # Website domain/source
        self.date = date  # Publication date if available
        self.result_type = result_type  # Type of result (regular, featured snippet, etc.)
        self.image_url = image_url  # Thumbnail image URL if available
        self.related_links = related_links or []  # Related links if available
        self.page_content = page_content or {}  # Full page content if fetched
        self.snippet_blocks = snippet_blocks or []  # Additional text snippets from search results

    def __repr__(self):
        return f"SearchResult(url={self.url}, title={self.title}, description={self.description}, source={self.source}, date={self.date})"

    def get_domain(self):
        """Extract the domain from the URL"""
        try:
            return urlparse(self.url).netloc
        except:
            return ""

def search(term, num_results=10, lang="en", proxy=None, advanced=False, sleep_interval=0, 
           timeout=5, safe="active", ssl_verify=None, region=None, start_num=0, unique=False, 
           fetch_content=False):
    """
    Search the Google search engine
    
    Args:
        ...existing arguments...
        fetch_content: If True, fetch and parse the full text content of each search result
    """
    # Proxy setup
    proxies = {"https": proxy, "http": proxy} if proxy and (proxy.startswith("https") or proxy.startswith("http")) else None

    start = start_num
    fetched_results = 0  # Keep track of the total fetched results
    fetched_links = set() # to keep track of links that are already seen previously

    while fetched_results < num_results:
        # Send request
        resp = _req(term, num_results - start,
                    lang, start, proxies, timeout, safe, ssl_verify, region)
        
        # put in file - comment for debugging purpose
        # with open('google.html', 'w') as f:
        #     f.write(resp.text)
        
        # Parse
        soup = BeautifulSoup(resp.text, "html.parser")
        result_block = soup.find_all("div", class_="ezO2md")
        new_results = 0  # Keep track of new results in this iteration

        for result in result_block:
            # Find the link tag within the result block
            link_tag = result.find("a", href=True)
            # Find the title tag within the link tag
            title_tag = link_tag.find("span", class_="CVA68e") if link_tag else None
            # Find the description tag within the result block
            description_tag = result.find("span", class_="FrIlee")

            # Check if all necessary tags are found
            if link_tag and title_tag and description_tag:
                # Extract and decode the link URL
                link = unquote(link_tag["href"].split("&")[0].replace("/url?q=", "")) if link_tag else ""
                
                # Extract additional information
                # Try to find source/domain info (typically shown in green text)
                source_tag = result.find("span", class_="qXLe6d")
                source = source_tag.text if source_tag else ""
                
                # Try to extract date if available (often near source)
                date_tag = result.find("span", class_="MUxGbd")
                date = date_tag.text if date_tag else ""
                
                # Check for any thumbnail image
                image_tag = result.find("img", class_="XNo5Ab")
                image_url = image_tag["src"] if image_tag and "src" in image_tag.attrs else None
                
                # Look for result type (featured snippet, news, etc.)
                result_type = "regular"
                if result.find("div", class_="UDZeY"):
                    result_type = "featured"
                elif result.find("div", class_="tcPEUc"):
                    result_type = "news"
                
                # Look for related links within this result
                related_links = []
                related_tags = result.find_all("a", class_="k8XOCe")
                for r_tag in related_tags:
                    if r_tag and "href" in r_tag.attrs:
                        r_url = unquote(r_tag["href"].split("&")[0].replace("/url?q=", ""))
                        r_text = r_tag.text if r_tag.text else ""
                        related_links.append({"url": r_url, "text": r_text})
                
                # Extract any additional text snippets from the search result
                snippet_blocks = []
                
                # Look for additional text blocks like featured snippets, site links, etc.
                for text_block in result.find_all(["div", "span"], class_=re.compile(r"VwiC3b|yXK7lf|MUxGbd")):
                    if text_block and text_block.get_text(strip=True) and text_block != description_tag:
                        snippet_text = text_block.get_text(strip=True)
                        if snippet_text and len(snippet_text) > 10 and snippet_text not in snippet_blocks:
                            snippet_blocks.append(snippet_text)
                
                # Extract table data if present
                tables = result.find_all("table")
                for table in tables:
                    rows = []
                    for tr in table.find_all("tr"):
                        row = [td.get_text(strip=True) for td in tr.find_all(["td", "th"])]
                        if row:
                            rows.append(row)
                    if rows:
                        snippet_blocks.append(f"Table data: {rows}")
                
                # Check if the link has already been fetched and if unique results are required
                if link in fetched_links and unique:
                    continue  # Skip this result if the link is not unique
                
                # Add the link to the set of fetched links
                fetched_links.add(link)
                
                # Extract the title text
                title = title_tag.text if title_tag else ""
                
                # Extract the description text
                description = description_tag.text if description_tag else ""
                
                # Fetch the full page content if requested
                page_content = None
                if fetch_content and link.startswith(("http://", "https://")):
                    try:
                        print(f"Fetching content from: {link}")
                        page_content = fetch_page_content(link, timeout)
                        sleep(sleep_interval)  # Be nice to the servers
                    except Exception as e:
                        print(f"Error fetching content: {str(e)}")
                
                # Increment the count of fetched results
                fetched_results += 1
                
                # Increment the count of new results in this iteration
                new_results += 1
                
                # Yield the result based on the advanced flag
                if advanced:
                    yield SearchResult(
                        link, 
                        title, 
                        description, 
                        source=source, 
                        date=date, 
                        result_type=result_type, 
                        image_url=image_url, 
                        related_links=related_links,
                        page_content=page_content,
                        snippet_blocks=snippet_blocks
                    )
                else:
                    yield link

            if fetched_results >= num_results:
                break  # Stop if we have fetched the desired number of results

        if new_results == 0:
            #If you want to have printed to your screen that the desired amount of queries can not been fulfilled, uncomment the line below:
            #print(f"Only {fetched_results} results found for query requiring {num_results} results. Moving on to the next query.")
            break  # Break the loop if no new results were found in this iteration

        start += 10  # Prepare for the next set of results
        sleep(sleep_interval)


class GoogleParam(ComponentParamBase):
    """
    Define the Google component parameters.
    """

    def __init__(self):
        super().__init__()
        self.top_n = 10
        self.api_key = "xxx"
        self.country = "cn"
        self.language = "en"
        self.provider = "OpenSearch" 

    def check(self):
        self.check_positive_integer(self.top_n, "Top N")
        self.check_empty(self.api_key, "API key")
        self.check_valid_value(self.country, "Google Country",
                               ['af', 'al', 'dz', 'as', 'ad', 'ao', 'ai', 'aq', 'ag', 'ar', 'am', 'aw', 'au', 'at',
                                'az', 'bs', 'bh', 'bd', 'bb', 'by', 'be', 'bz', 'bj', 'bm', 'bt', 'bo', 'ba', 'bw',
                                'bv', 'br', 'io', 'bn', 'bg', 'bf', 'bi', 'kh', 'cm', 'ca', 'cv', 'ky', 'cf', 'td',
                                'cl', 'cn', 'cx', 'cc', 'co', 'km', 'cg', 'cd', 'ck', 'cr', 'ci', 'hr', 'cu', 'cy',
                                'cz', 'dk', 'dj', 'dm', 'do', 'ec', 'eg', 'sv', 'gq', 'er', 'ee', 'et', 'fk', 'fo',
                                'fj', 'fi', 'fr', 'gf', 'pf', 'tf', 'ga', 'gm', 'ge', 'de', 'gh', 'gi', 'gr', 'gl',
                                'gd', 'gp', 'gu', 'gt', 'gn', 'gw', 'gy', 'ht', 'hm', 'va', 'hn', 'hk', 'hu', 'is',
                                'in', 'id', 'ir', 'iq', 'ie', 'il', 'it', 'jm', 'jp', 'jo', 'kz', 'ke', 'ki', 'kp',
                                'kr', 'kw', 'kg', 'la', 'lv', 'lb', 'ls', 'lr', 'ly', 'li', 'lt', 'lu', 'mo', 'mk',
                                'mg', 'mw', 'my', 'mv', 'ml', 'mt', 'mh', 'mq', 'mr', 'mu', 'yt', 'mx', 'fm', 'md',
                                'mc', 'mn', 'ms', 'ma', 'mz', 'mm', 'na', 'nr', 'np', 'nl', 'an', 'nc', 'nz', 'ni',
                                'ne', 'ng', 'nu', 'nf', 'mp', 'no', 'om', 'pk', 'pw', 'ps', 'pa', 'pg', 'py', 'pe',
                                'ph', 'pn', 'pl', 'pt', 'pr', 'qa', 're', 'ro', 'ru', 'rw', 'sh', 'kn', 'lc', 'pm',
                                'vc', 'ws', 'sm', 'st', 'sa', 'sn', 'rs', 'sc', 'sl', 'sg', 'sk', 'si', 'sb', 'so',
                                'za', 'gs', 'es', 'lk', 'sd', 'sr', 'sj', 'sz', 'se', 'ch', 'sy', 'tw', 'tj', 'tz',
                                'th', 'tl', 'tg', 'tk', 'to', 'tt', 'tn', 'tr', 'tm', 'tc', 'tv', 'ug', 'ua', 'ae',
                                'uk', 'gb', 'us', 'um', 'uy', 'uz', 'vu', 've', 'vn', 'vg', 'vi', 'wf', 'eh', 'ye',
                                'zm', 'zw'])
        self.check_valid_value(self.language, "Google languages",
                               ['af', 'ak', 'sq', 'ws', 'am', 'ar', 'hy', 'az', 'eu', 'be', 'bem', 'bn', 'bh',
                                'xx-bork', 'bs', 'br', 'bg', 'bt', 'km', 'ca', 'chr', 'ny', 'zh-cn', 'zh-tw', 'co',
                                'hr', 'cs', 'da', 'nl', 'xx-elmer', 'en', 'eo', 'et', 'ee', 'fo', 'tl', 'fi', 'fr',
                                'fy', 'gaa', 'gl', 'ka', 'de', 'el', 'kl', 'gn', 'gu', 'xx-hacker', 'ht', 'ha', 'haw',
                                'iw', 'hi', 'hu', 'is', 'ig', 'id', 'ia', 'ga', 'it', 'ja', 'jw', 'kn', 'kk', 'rw',
                                'rn', 'xx-klingon', 'kg', 'ko', 'kri', 'ku', 'ckb', 'ky', 'lo', 'la', 'lv', 'ln', 'lt',
                                'loz', 'lg', 'ach', 'mk', 'mg', 'ms', 'ml', 'mt', 'mv', 'mi', 'mr', 'mfe', 'mo', 'mn',
                                'sr-me', 'my', 'ne', 'pcm', 'nso', 'no', 'nn', 'oc', 'or', 'om', 'ps', 'fa',
                                'xx-pirate', 'pl', 'pt', 'pt-br', 'pt-pt', 'pa', 'qu', 'ro', 'rm', 'nyn', 'ru', 'gd',
                                'sr', 'sh', 'st', 'tn', 'crs', 'sn', 'sd', 'si', 'sk', 'sl', 'so', 'es', 'es-419', 'su',
                                'sw', 'sv', 'tg', 'ta', 'tt', 'te', 'th', 'ti', 'to', 'lua', 'tum', 'tr', 'tk', 'tw',
                                'ug', 'uk', 'ur', 'uz', 'vu', 'vi', 'cy', 'wo', 'xh', 'yi', 'yo', 'zu']
                               )
        self.check_valid_value(self.provider, "Provider type", ['SerpApi', 'GoogleCustomSearch','OpenSearch'])  

class Google(ComponentBase, ABC):
    component_name = "Google"

    def _run(self, history, **kwargs):
        ans = self.get_input()
        ans = " - ".join(ans["content"]) if "content" in ans else ""
        if not ans:
            return Google.be_output("")
        try:
            if self._param.provider == "SerpApi":
                google_res = self.search_serpapi(ans)
            elif self._param.provider == "GoogleCustomSearch":
                google_res = self.search_google_custom(ans)
            elif self._param.provider == "OpenSearch":
                google_res = self.search_opensearch(ans)
            else:
                return Google.be_output("**ERROR**: Unsupported provider!")
        except Exception as e:
            return Google.be_output(f"**ERROR**: {e}!")

        if not google_res:
            return Google.be_output("")

        df = pd.DataFrame(google_res)
        logging.debug(f"df: {df}")
        return df

    def search_serpapi(self, query):
        """
        Perform a search using the SerpApi and return the results.
        """
        client = SerpApiSearch(
            {"engine": "google", "q": query, "api_key": self._param.api_key, "gl": self._param.country,
             "hl": self._param.language, "num": self._param.top_n})
        results = [{"content": '<a href="' + i["link"] + '">' + i["title"] + '</a>    ' + i["snippet"]} for i in
                   client.get_dict()["organic_results"]]

        return results

    def search_google_custom(self, query):
        """
        Perform a search using the Google Custom Search API and return the results.
        """
        url = f"https://www.googleapis.com/customsearch/v1?q={query}&key={self._param.api_key}&cx=YOUR_CX_ID&gl={self._param.country}&hl={self._param.language}&num={self._param.top_n}"
        response = requests.get(url)
        response.raise_for_status()
        data = response.json()
        results = [{"content": '<a href="' + item["link"] + '">' + item["title"] + '</a>    ' + item["snippet"]} for item in data.get("items", [])]

        return results

    def search_opensearch(self, query):
        """
        Perform a search using the OpenSearch and return the results.
        """

        results = []
        for url in search(query, num_results=self._param.top_n, lang=self._param.language, advanced=True ):
            try:
                title = url.title
                snippet = url.description if url.description else 'No description available'
                snippet += f'{url.page_content}' if url.page_content else ''
                results.append({"content": f'<a href="{url.url}">{title}</a>{snippet}'})
            except Exception as e:
                logging.error(f"Error processing search result {url}: {e}")
                results.append({"content": f'<a href="{url.url}">{url.url}</a>    Error processing details'})
        return results
