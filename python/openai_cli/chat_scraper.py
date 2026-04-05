#!/usr/bin/env python3
"""
ChatGPT Conversation Scraper
Scrapes and archives chat conversations from OpenAI's ChatGPT interface
"""

import os
import time
import json
import hashlib
import logging
from datetime import datetime
from typing import List, Dict, Any, Optional
from selenium import webdriver
from selenium.webdriver.chrome.options import Options
from selenium.webdriver.common.by import By
from selenium.webdriver.support.ui import WebDriverWait
from selenium.webdriver.support import expected_conditions as EC
from selenium.common.exceptions import TimeoutException, WebDriverException
from config import Config
from db.database import DatabaseManager

logger = logging.getLogger(__name__)

class ChatScraper:
    def __init__(self, config: Config, db: DatabaseManager):
        self.config = config
        self.db = db
        self.driver: Optional[webdriver.Chrome] = None
        self._ensure_directories()
    
    def _ensure_directories(self):
        """Ensure chat download directories exist"""
        os.makedirs(self.config.chats_dir, exist_ok=True)
        logger.info(f"Chat download directory ensured: {self.config.chats_dir}")
    
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
    
    def login_and_scrape_conversations(self) -> List[Dict[str, Any]]:
        """Login to ChatGPT and scrape conversation metadata"""
        if not self.driver:
            self.driver = self.setup_browser()
        
        try:
            logger.info("Navigating to ChatGPT...")
            self.driver.get("https://chat.openai.com/")
            
            # Wait for user to login manually
            input("🔐 Please log into ChatGPT manually and press Enter to continue...")
            
            # Wait for page to load
            time.sleep(5)
            
            # Wait for conversations to load
            wait = WebDriverWait(self.driver, 30)
            wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "[data-testid='conversation-turn-2']")))
            
            # Get all conversation links
            conversation_elements = self.driver.find_elements(By.CSS_SELECTOR, "a[href*='/c/']")
            conversations = []
            
            for element in conversation_elements:
                try:
                    href = element.get_attribute("href")
                    title = element.text.strip()
                    
                    if href and title:
                        conversation_id = href.split('/c/')[-1]
                        conversations.append({
                            'conversation_id': conversation_id,
                            'title': title,
                            'url': href
                        })
                        
                except Exception as e:
                    logger.warning(f"Failed to extract conversation metadata: {e}")
                    continue
            
            logger.info(f"Found {len(conversations)} conversations to process")
            return conversations
            
        except TimeoutException:
            logger.error("Timeout waiting for page to load")
            return []
        except Exception as e:
            logger.error(f"Failed to scrape conversations: {e}")
            return []
    
    def scrape_conversation_details(self, conversation_data: Dict[str, Any]) -> Optional[Dict[str, Any]]:
        """Scrape detailed conversation content"""
        if not self.driver:
            logger.error("Browser driver not initialized")
            return None
            
        try:
            logger.info(f"Scraping conversation: {conversation_data['title']}")
            
            # Navigate to the conversation
            self.driver.get(conversation_data['url'])
            time.sleep(3)
            
            # Wait for messages to load
            wait = WebDriverWait(self.driver, 30)
            wait.until(EC.presence_of_element_located((By.CSS_SELECTOR, "[data-testid='conversation-turn-2']")))
            
            # Extract all messages
            message_elements = self.driver.find_elements(By.CSS_SELECTOR, "[data-testid^='conversation-turn-']")
            messages = []
            total_tokens = 0
            
            for element in message_elements:
                try:
                    # Determine role (user or assistant)
                    role = "assistant"  # Default
                    if element.find_elements(By.CSS_SELECTOR, "[data-testid='user']"):
                        role = "user"
                    
                    # Extract content
                    content_element = element.find_element(By.CSS_SELECTOR, "[data-message-author-role]")
                    content = content_element.text.strip()
                    
                    if content:
                        message_id = hashlib.md5(f"{conversation_data['conversation_id']}_{role}_{content[:100]}".encode()).hexdigest()
                        
                        messages.append({
                            'message_id': message_id,
                            'role': role,
                            'content': content,
                            'timestamp': datetime.now().isoformat(),
                            'tokens_used': len(content.split()) * 1.3,  # Rough estimate
                            'model_used': 'gpt-4',  # Default assumption
                            'metadata': {}
                        })
                        
                        total_tokens += len(content.split()) * 1.3
                        
                except Exception as e:
                    logger.warning(f"Failed to extract message: {e}")
                    continue
            
            # Generate summary
            summary = self._generate_summary(messages)
            
            return {
                'conversation_id': conversation_data['conversation_id'],
                'title': conversation_data['title'],
                'messages': messages,
                'message_count': len(messages),
                'total_tokens': int(total_tokens),
                'summary': summary,
                'participants': ['user', 'assistant'],
                'model_used': 'gpt-4'
            }
            
        except Exception as e:
            logger.error(f"Failed to scrape conversation details: {e}")
            return None
    
    def _generate_summary(self, messages: List[Dict[str, Any]]) -> str:
        """Generate a summary of the conversation"""
        if not messages:
            return "Empty conversation"
        
        # Take first few messages to create a summary
        summary_messages = messages[:3]
        summary_text = " ".join([msg['content'][:100] for msg in summary_messages])
        
        if len(summary_text) > 200:
            summary_text = summary_text[:200] + "..."
        
        return summary_text
    
    def save_conversation_to_file(self, conversation_data: Dict[str, Any]) -> Optional[str]:
        """Save conversation to JSON file"""
        try:
            filename = f"{conversation_data['conversation_id']}_{datetime.now().strftime('%Y%m%d_%H%M%S')}.json"
            filepath = os.path.join(self.config.chats_dir, filename)
            
            # Prepare data for JSON export
            export_data = {
                'conversation_id': conversation_data['conversation_id'],
                'title': conversation_data['title'],
                'created_at': datetime.now().isoformat(),
                'message_count': conversation_data['message_count'],
                'total_tokens': conversation_data['total_tokens'],
                'summary': conversation_data['summary'],
                'participants': conversation_data['participants'],
                'model_used': conversation_data['model_used'],
                'messages': conversation_data['messages']
            }
            
            with open(filepath, 'w', encoding='utf-8') as f:
                json.dump(export_data, f, indent=2, ensure_ascii=False)
            
            logger.info(f"Saved conversation to: {filename}")
            return filename
            
        except Exception as e:
            logger.error(f"Failed to save conversation to file: {e}")
            return None
    
    def archive_chats(self) -> Dict[str, Any]:
        """Main method to archive all chat conversations"""
        logger.info("Starting chat archive process...")
        
        try:
            # Scrape conversation metadata
            conversations = self.login_and_scrape_conversations()
            
            if not conversations:
                logger.warning("No conversations found to archive")
                return {'success': False, 'message': 'No conversations found'}
            
            # Process conversations
            successful_downloads = 0
            successful_db_inserts = 0
            
            for i, conversation_data in enumerate(conversations):
                logger.info(f"Processing conversation {i+1}/{len(conversations)}: {conversation_data['title']}")
                
                try:
                    # Scrape detailed conversation
                    detailed_conversation = self.scrape_conversation_details(conversation_data)
                    
                    if detailed_conversation:
                        # Save to file
                        filename = self.save_conversation_to_file(detailed_conversation)
                        
                        if filename:
                            successful_downloads += 1
                            
                            # Get file size
                            filepath = os.path.join(self.config.chats_dir, filename)
                            file_size = os.path.getsize(filepath) if os.path.exists(filepath) else 0
                            
                            # Insert into database
                            if self.db.insert_chat_metadata(
                                conversation_id=detailed_conversation['conversation_id'],
                                title=detailed_conversation['title'],
                                filename=filename,
                                message_count=detailed_conversation['message_count'],
                                total_tokens=detailed_conversation['total_tokens'],
                                model_used=detailed_conversation['model_used'],
                                participants=detailed_conversation['participants'],
                                summary=detailed_conversation['summary'],
                                file_size=file_size
                            ):
                                successful_db_inserts += 1
                                
                                # Insert individual messages
                                for message in detailed_conversation['messages']:
                                    self.db.insert_chat_message(
                                        conversation_id=detailed_conversation['conversation_id'],
                                        message_id=message['message_id'],
                                        role=message['role'],
                                        content=message['content'],
                                        tokens_used=message['tokens_used'],
                                        model_used=message['model_used'],
                                        metadata=message['metadata']
                                    )
                            
                        else:
                            logger.error(f"Failed to save conversation: {conversation_data['title']}")
                    
                except Exception as e:
                    logger.error(f"Failed to process conversation {conversation_data['title']}: {e}")
                
                # Batch processing delay
                if (i + 1) % self.config.batch_size == 0:
                    logger.info(f"Processed {i + 1} conversations, taking a short break...")
                    time.sleep(2)
            
            logger.info(f"Chat archive completed: {successful_downloads} downloaded, {successful_db_inserts} metadata inserted")
            
            return {
                'success': True,
                'total_found': len(conversations),
                'downloaded': successful_downloads,
                'metadata_inserted': successful_db_inserts
            }
            
        except Exception as e:
            logger.error(f"Chat archive process failed: {e}")
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