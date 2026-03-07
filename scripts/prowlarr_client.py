#!/usr/bin/env python3
"""
Prowlarr Adapter for GoStorm Sync Scripts
Converts Prowlarr/Newznab results to Stremio/Torrentio format.
"""

import json
import logging
import requests
from typing import List, Dict, Any, Optional

class ProwlarrClient:
    def __init__(self):
        self.API_KEY = "80b0e137663b4523bae737ec8f5fc791"
        self.BASE_URL = "http://192.168.1.250:9696"
        self.SEARCH_ENDPOINT = f"{self.BASE_URL}/api/v1/search"
        
    def fetch_from_prowlarr(self, imdb_id: str, content_type: str = "movie") -> List[Dict[str, Any]]:
        """
        Directly query Prowlarr API for a specific IMDB ID.
        """
        # Map Stremio 'series' to Prowlarr 'tv'
        prowlarr_type = "tv" if content_type == "series" else content_type
        
        params = {
            "apikey": self.API_KEY,
            "imdbId": imdb_id,
            "type": prowlarr_type
        }
        try:
            response = requests.get(self.SEARCH_ENDPOINT, params=params, timeout=45)
            if response.status_code == 200:
                return response.json()
            else:
                logging.warning(f"Prowlarr API returned status {response.status_code}")
        except Exception as e:
            logging.error(f"Error fetching from Prowlarr: {e}")
        return []

    def fetch_torrents(self, imdb_id: str, content_type: str = "movie") -> List[Dict[str, Any]]:
        """
        Fetch torrents from Prowlarr and return them in Stremio/Torrentio format.
        """
        prowlarr_results = self.fetch_from_prowlarr(imdb_id, content_type)
        return self._map_to_stremio_format(prowlarr_results)

    def _map_to_stremio_format(self, prowlarr_results: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """
        Maps Prowlarr results to Stremio/Torrentio 'streams' format.
        """
        streams = []
        for res in prowlarr_results:
            title = res.get("title", "")
            size_bytes = res.get("size", 0)
            seeders = res.get("seeders", 0)
            leechers = res.get("leechers", 0)
            info_hash = res.get("infoHash", "")
            indexer = res.get("indexer", "Prowlarr")
            
            if not info_hash:
                continue

            # Convert size to GB for the title string
            size_gb = size_bytes / (1024 * 1024 * 1024)
            
            # Format title to match Torrentio's multiline format (essential for existing regex)
            # Torrentio title looks like: "Movie.Title.2024.2160p.4K\n👤 123 ⬇️ 10\n23.28 GB"
            formatted_title = f"{title}\n👤 {seeders} ⬇️ {leechers}\n💾 {size_gb:.2f}GB"
            
            stream = {
                "name": f"Prowlarr\n{indexer}",
                "title": formatted_title,
                "infoHash": info_hash,
                "behaviorHints": {
                    "bingeGroup": f"prowlarr-{indexer}"
                }
            }
            streams.append(stream)
            
        return streams
