#!/usr/bin/env python3
"""
Sora Video Downloader
Scrapes and downloads Sora-generated videos from OpenAI's Sora interface
"""

import os
import time
import json
import hashlib
import logging
import requests
from datetime import datetime
from typing import List, Dict, Any, Optional
from urllib.parse import urlparse
from selenium import webdriver
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.common.exceptions import TimeoutException, WebDriverException
from config import Config
from db.database import DatabaseManager

logger = logging.getLogger(__name__)

class SoraDownloader:
    def __init__(self, config: Config, db: DatabaseManager):
        self.config = config
        self.db = db
        self.driver: Optional[webdriver.Chrome] = None
        self._ensure_directories()
    
    def _ensure_directories(self):
        """Ensure video download directories exist"""
        os.makedirs(self.config.videos_dir, exist_ok=True)
        logger.info(f"Video download directory ensured: {self.config.videos_dir}")
    
    def setup_browser(self) -> webdriver.Chrome:
        """Setup Chrome browser with configured options"""
        chrome_options = Options()
        
        if self.config.chrome_headless:
            chrome_options.add_argument("--headless=new")
        
        if self.config.chrome_disable_gpu:
            chrome_options.add_argument("--disable-gpu")
        
        chrome_options.add_argument(f"--window-size={self.config.chrome_window_size}")
        chrome_options.add_argument("--no-sandbox")
        chrome_options.add_argument("--disable-dev-shm-usage")
        chrome_options.add_argument("--disable-blink-features=AutomationControlled")
        chrome_options.add_experimental_option("excludeSwitches", ["enable-automation"])
        chrome_options.add_experimental_option('useAutomationExtension', False)
        
        try:
            driver = webdriver.Chrome(options=chrome_options)
            driver.execute_script("Object.defineProperty(navigator, 'webdriver', {get: () => undefined})")
            logger.info("Chrome browser initialized successfully")
            return driver
        except Exception as e:
            logger.error(f"Failed to initialize Chrome browser: {e}")
            raise
    
    def login_and_scrape_videos(self) -> List[Dict[str, Any]]:
        """Login to Sora and scrape video metadata"""
        if not self.driver:
            self.driver = self.setup_browser()
        
        try:
            logger.info("Navigating to OpenAI Sora...")
            if self.driver:
                self.driver.get("https://openai.com/sora")
            
            # Wait for user to login manually
            input("🔐 Please log into OpenAI Sora manually and press Enter to continue...")
            
            # Wait for page to load
            time.sleep(5)
            
            # Wait for videos to load
            if self.driver:
                wait = WebDriverWait(self.driver, 30)
                wait.until(EC.presence_of_element_located((By.TAG_NAME, "video")))
                
                # Extract video elements
                video_elements = self.driver.find_elements(By.TAG_NAME, "video")
                video_data = []
                
                for video in video_elements:
                    try:
                        src = video.get_attribute("src")
                        if src and "blob:" not in src:
                            # Extract metadata from video element or parent
                            title = video.get_attribute("title") or ""
                            alt_text = video.get_attribute("alt") or ""
                            
                            # Generate video ID from URL
                            video_id = hashlib.md5(src.encode()).hexdigest()
                            
                            # Extract filename from URL
                            filename = os.path.basename(urlparse(src).path)
                            if not filename or '.' not in filename:
                                filename = f"{video_id}.mp4"
                            
                            # Get video duration and other attributes
                            duration = video.get_attribute("duration")
                            width = video.get_attribute("width")
                            height = video.get_attribute("height")
                            
                            # Try to find associated text/prompt
                            prompt_text = self._extract_prompt_text(video)
                            
                            video_data.append({
                                'video_id': video_id,
                                'url': src,
                                'filename': filename,
                                'title': title or alt_text,
                                'duration': float(duration) if duration else None,
                                'resolution': f"{width}x{height}" if width and height else None,
                                'prompt_text': prompt_text
                            })
                            
                    except Exception as e:
                        logger.warning(f"Failed to extract metadata from video: {e}")
                        continue
                
                logger.info(f"Found {len(video_data)} videos to process")
                return video_data
            else:
                logger.error("Driver not initialized")
                return []
            
        except TimeoutException:
            logger.error("Timeout waiting for page to load")
            return []
        except Exception as e:
            logger.error(f"Failed to scrape videos: {e}")
            return []
    
    def _extract_prompt_text(self, video_element) -> str:
        """Extract prompt text associated with the video"""
        try:
            # Look for nearby text elements that might contain the prompt
            parent = video_element.find_element(By.XPATH, "./..")
            
            # Try to find text elements near the video
            text_elements = parent.find_elements(By.XPATH, ".//p | .//div | .//span")
            
            for element in text_elements:
                text = element.text.strip()
                if text and len(text) > 10:  # Likely a prompt if it's substantial text
                    return text
            
            # If no text found, try looking for data attributes
            prompt_attr = video_element.get_attribute("data-prompt") or video_element.get_attribute("aria-label")
            if prompt_attr:
                return prompt_attr
            
            return ""
            
        except Exception as e:
            logger.warning(f"Failed to extract prompt text: {e}")
            return ""
    
    def download_video_with_retry(self, video_data: Dict[str, Any]) -> bool:
        """Download video with retry logic"""
        url = video_data['url']
        filename = video_data['filename']
        filepath = os.path.join(self.config.videos_dir, filename)
        
        # Skip if file already exists
        if os.path.exists(filepath):
            logger.info(f"Video already exists: {filename}")
            return True
        
        for attempt in range(self.config.max_retries):
            try:
                logger.info(f"Downloading video {filename} (attempt {attempt + 1}/{self.config.max_retries})")
                
                response = requests.get(url, stream=True, timeout=self.config.download_timeout)
                response.raise_for_status()
                
                with open(filepath, 'wb') as f:
                    for chunk in response.iter_content(chunk_size=8192):
                        f.write(chunk)
                
                logger.info(f"Successfully downloaded: {filename}")
                return True
                
            except requests.exceptions.RequestException as e:
                logger.warning(f"Download attempt {attempt + 1} failed for {filename}: {e}")
                if attempt < self.config.max_retries - 1:
                    time.sleep(2 ** attempt)  # Exponential backoff
                else:
                    logger.error(f"Failed to download {filename} after {self.config.max_retries} attempts")
                    return False
        
        return False
    
    def _extract_video_metadata(self, filepath: str) -> Dict[str, Any]:
        """Extract metadata from downloaded video file"""
        try:
            # Try to import cv2, but don't fail if it's not available
            try:
                import cv2
                cv2_available = True
            except ImportError:
                cv2_available = False
                logger.warning("OpenCV not available, using basic file info")
            
            if cv2_available:
                cap = cv2.VideoCapture(filepath)
                
                # Get video properties
                fps = cap.get(cv2.CAP_PROP_FPS)
                frame_count = cap.get(cv2.CAP_PROP_FRAME_COUNT)
                width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
                height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
                
                # Calculate duration
                duration = frame_count / fps if fps > 0 else 0
                
                cap.release()
                
                return {
                    'fps': fps,
                    'frame_count': frame_count,
                    'width': width,
                    'height': height,
                    'duration': duration,
                    'resolution': f"{width}x{height}"
                }
            else:
                return {}
            
        except Exception as e:
            logger.warning(f"Failed to extract video metadata: {e}")
            return {}
    
    def archive_videos(self) -> Dict[str, Any]:
        """Main method to archive all Sora videos"""
        logger.info("Starting Sora video archive process...")
        
        try:
            # Scrape video metadata
            video_data_list = self.login_and_scrape_videos()
            
            if not video_data_list:
                logger.warning("No videos found to archive")
                return {'success': False, 'message': 'No videos found'}
            
            # Process videos in batches
            successful_downloads = 0
            successful_db_inserts = 0
            
            for i, video_data in enumerate(video_data_list):
                logger.info(f"Processing video {i+1}/{len(video_data_list)}: {video_data['filename']}")
                
                # Download video
                if self.download_video_with_retry(video_data):
                    successful_downloads += 1
                    
                    # Get file size
                    filepath = os.path.join(self.config.videos_dir, video_data['filename'])
                    file_size = os.path.getsize(filepath) if os.path.exists(filepath) else 0
                    
                    # Extract additional metadata from the file
                    video_metadata = self._extract_video_metadata(filepath)
                    
                    # Insert metadata into database
                    try:
                        # Generate tags from prompt text
                        tags = []
                        if video_data.get('prompt_text'):
                            tags = [video_data['prompt_text']]
                        
                        # Use extracted metadata or fallback to scraped data
                        duration = video_metadata.get('duration') or video_data.get('duration', 0)
                        resolution = video_metadata.get('resolution') or video_data.get('resolution', 'Unknown')
                        
                        if self.db.insert_video_metadata(
                            title=video_data['title'],
                            filename=video_data['filename'],
                            url=video_data['url'],
                            duration_seconds=int(duration) if duration else 0,
                            resolution=resolution,
                            tags=tags
                        ):
                            successful_db_inserts += 1
                        
                    except Exception as e:
                        logger.error(f"Failed to insert metadata for {video_data['filename']}: {e}")
                
                # Batch processing delay
                if (i + 1) % self.config.batch_size == 0:
                    logger.info(f"Processed {i + 1} videos, taking a short break...")
                    time.sleep(2)
            
            logger.info(f"Sora video archive completed: {successful_downloads} downloaded, {successful_db_inserts} metadata inserted")
            
            return {
                'success': True,
                'total_found': len(video_data_list),
                'downloaded': successful_downloads,
                'metadata_inserted': successful_db_inserts
            }
            
        except Exception as e:
            logger.error(f"Sora video archive process failed: {e}")
            return {'success': False, 'message': str(e)}
        
        finally:
            if self.driver:
                self.driver.quit()
                logger.info("Chrome browser closed")
    
    def close(self):
        """Clean up resources"""
        if self.driver:
            self.driver.quit()
            logger.info("Chrome browser closed")

# Legacy function for backward compatibility
def archive_videos():
    """
    Legacy function for backward compatibility
    """
    logger.warning("Using legacy archive_videos function - consider using SoraDownloader class")
    
    # This would need proper initialization
    # For now, just log that it's not implemented
    logger.info("Sora video archiving not yet implemented")
    return True 