#!/usr/bin/env python3
import os
import time
import requests
import hashlib
import logging
from urllib.parse import urlparse
from selenium import webdriver
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.common.exceptions import TimeoutException, WebDriverException
from typing import List, Dict, Any, Optional
from config import Config
from db.database import DatabaseManager

logger = logging.getLogger(__name__)

class MediaScraper:
    def __init__(self, config: Config, db: DatabaseManager):
        self.config = config
        self.db = db
        self.driver = None
        self._ensure_directories()
    
    def _ensure_directories(self):
        """Ensure download directories exist"""
        os.makedirs(self.config.images_dir, exist_ok=True)
        os.makedirs(self.config.videos_dir, exist_ok=True)
        logger.info(f"Download directories ensured: {self.config.images_dir}, {self.config.videos_dir}")
    
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
    
    def login_and_scrape_images(self) -> List[Dict[str, Any]]:
        """Login to ChatGPT and scrape image metadata"""
        if not self.driver:
            self.driver = self.setup_browser()
        
        try:
            logger.info("Navigating to ChatGPT media library...")
            self.driver.get("https://chat.openai.com/media-library")
            
            # Wait for user to login manually
            input("🔐 Please log into ChatGPT manually and press Enter to continue...")
            
            # Wait for page to load
            time.sleep(5)
            
            # Wait for images to load
            wait = WebDriverWait(self.driver, 30)
            wait.until(EC.presence_of_element_located((By.TAG_NAME, "img")))
            
            # Extract image elements
            images = self.driver.find_elements(By.TAG_NAME, "img")
            image_data = []
            
            for img in images:
                src = img.get_attribute("src")
                if src and "blob:" not in src:
                    # Extract metadata from image element or parent
                    try:
                        # Try to get alt text or title
                        alt_text = img.get_attribute("alt") or ""
                        title = img.get_attribute("title") or ""
                        
                        # Generate image ID from URL
                        image_id = hashlib.md5(src.encode()).hexdigest()
                        
                        # Extract filename from URL
                        filename = os.path.basename(urlparse(src).path)
                        if not filename or '.' not in filename:
                            filename = f"{image_id}.jpg"
                        
                        image_data.append({
                            'image_id': image_id,
                            'url': src,
                            'filename': filename,
                            'alt_text': alt_text,
                            'title': title
                        })
                        
                    except Exception as e:
                        logger.warning(f"Failed to extract metadata from image: {e}")
                        continue
            
            logger.info(f"Found {len(image_data)} images to process")
            return image_data
            
        except TimeoutException:
            logger.error("Timeout waiting for page to load")
            return []
        except Exception as e:
            logger.error(f"Failed to scrape images: {e}")
            return []
    
    def download_image_with_retry(self, image_data: Dict[str, Any]) -> bool:
        """Download image with retry logic"""
        url = image_data['url']
        filename = image_data['filename']
        filepath = os.path.join(self.config.images_dir, filename)
        
        # Skip if file already exists
        if os.path.exists(filepath):
            logger.info(f"Image already exists: {filename}")
            return True
        
        for attempt in range(self.config.max_retries):
            try:
                logger.info(f"Downloading image {filename} (attempt {attempt + 1}/{self.config.max_retries})")
                
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
    
    def archive_images(self) -> Dict[str, Any]:
        """Main method to archive all images"""
        logger.info("Starting image archive process...")
        
        try:
            # Scrape image metadata
            image_data_list = self.login_and_scrape_images()
            
            if not image_data_list:
                logger.warning("No images found to archive")
                return {'success': False, 'message': 'No images found'}
            
            # Process images in batches
            successful_downloads = 0
            successful_db_inserts = 0
            
            for i, image_data in enumerate(image_data_list):
                logger.info(f"Processing image {i+1}/{len(image_data_list)}: {image_data['filename']}")
                
                # Download image
                if self.download_image_with_retry(image_data):
                    successful_downloads += 1
                    
                    # Insert metadata into database
                    try:
                        # Generate prompt hash from alt text or title
                        prompt_text = image_data.get('alt_text', '') or image_data.get('title', '')
                        prompt_hash = hashlib.md5(prompt_text.encode()).hexdigest()
                        
                        # Insert into database
                        if self.db.insert_image_metadata(
                            image_id=image_data['image_id'],
                            filename=image_data['filename'],
                            prompt_hash=prompt_hash,
                            created_at=time.strftime('%Y-%m-%d %H:%M:%S'),
                            tags=[prompt_text] if prompt_text else []
                        ):
                            successful_db_inserts += 1
                        
                    except Exception as e:
                        logger.error(f"Failed to insert metadata for {image_data['filename']}: {e}")
                
                # Batch processing delay
                if (i + 1) % self.config.batch_size == 0:
                    logger.info(f"Processed {i + 1} images, taking a short break...")
                    time.sleep(2)
            
            logger.info(f"Image archive completed: {successful_downloads} downloaded, {successful_db_inserts} metadata inserted")
            
            return {
                'success': True,
                'total_found': len(image_data_list),
                'downloaded': successful_downloads,
                'metadata_inserted': successful_db_inserts
            }
            
        except Exception as e:
            logger.error(f"Image archive process failed: {e}")
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
