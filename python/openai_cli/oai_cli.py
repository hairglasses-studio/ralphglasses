#!/usr/bin/env python3
import argparse
import logging
import sys
from typing import Dict, Any
from config import Config, setup_logging
from db.database import DatabaseManager
from media_scraper import MediaScraper

# Import scrapers
try:
    from sora_downloader import SoraDownloader
except ImportError:
    SoraDownloader = None

try:
    from chat_scraper import ChatScraper
except ImportError:
    ChatScraper = None

logger = logging.getLogger(__name__)

class OAICLI:
    def __init__(self):
        self.config = None
        self.db = None
        self.media_scraper = None
        self.sora_downloader = None
        self.chat_scraper = None
    
    def initialize(self):
        """Initialize the CLI with configuration and database"""
        try:
            # Load and validate configuration
            self.config = Config.from_env()
            self.config.validate()
            
            # Setup logging
            setup_logging(self.config)
            logger.info("OAI CLI initialized successfully")
            
            # Initialize database
            self.db = DatabaseManager(self.config)
            
            # Initialize scrapers
            self.media_scraper = MediaScraper(self.config, self.db)
            
            if SoraDownloader:
                self.sora_downloader = SoraDownloader(self.config, self.db)
            
            if ChatScraper:
                self.chat_scraper = ChatScraper(self.config, self.db)
            
            return True
            
        except Exception as e:
            logger.error(f"Failed to initialize CLI: {e}")
            return False
    
    def archive_images(self) -> Dict[str, Any]:
        """Archive images from OpenAI"""
        logger.info("Starting image archive process...")
        
        if not self.media_scraper:
            return {'success': False, 'message': 'Media scraper not available'}
        
        try:
            result = self.media_scraper.archive_images()
            logger.info(f"Image archive completed: {result}")
            return result
            
        except Exception as e:
            logger.error(f"Image archive failed: {e}")
            return {'success': False, 'message': str(e)}
    
    def archive_videos(self) -> Dict[str, Any]:
        """Archive videos from OpenAI"""
        logger.info("Starting video archive process...")
        
        if not self.sora_downloader:
            return {'success': False, 'message': 'Sora downloader not available'}
        
        try:
            result = self.sora_downloader.archive_videos()
            logger.info(f"Video archive completed: {result}")
            return result
            
        except Exception as e:
            logger.error(f"Video archive failed: {e}")
            return {'success': False, 'message': str(e)}
    
    def archive_chats(self) -> Dict[str, Any]:
        """Archive chat conversations from OpenAI"""
        logger.info("Starting chat archive process...")
        
        if not self.chat_scraper:
            return {'success': False, 'message': 'Chat scraper not available'}
        
        try:
            result = self.chat_scraper.archive_chats()
            logger.info(f"Chat archive completed: {result}")
            return result
            
        except Exception as e:
            logger.error(f"Chat archive failed: {e}")
            return {'success': False, 'message': str(e)}
    
    def archive_all(self) -> Dict[str, Any]:
        """Archive images, videos, and chats"""
        logger.info("Starting full archive process...")
        
        results = {
            'images': self.archive_images(),
            'videos': self.archive_videos(),
            'chats': self.archive_chats()
        }
        
        # Determine overall success
        overall_success = all(result['success'] for result in results.values())
        
        logger.info(f"Full archive completed - Images: {results['images']['success']}, Videos: {results['videos']['success']}, Chats: {results['chats']['success']}")
        
        return {
            'success': overall_success,
            'results': results
        }
    
    def get_stats(self) -> Dict[str, Any]:
        """Get archive statistics"""
        try:
            stats = self.db.get_archive_stats()
            return {'success': True, 'stats': stats}
        except Exception as e:
            logger.error(f"Failed to get stats: {e}")
            return {'success': False, 'message': str(e)}
    
    def cleanup(self):
        """Clean up resources"""
        if self.media_scraper:
            self.media_scraper.close()
        
        if self.sora_downloader:
            self.sora_downloader.close()
        
        if self.chat_scraper:
            self.chat_scraper.close()
        
        if self.db:
            self.db.close()
        
        logger.info("CLI cleanup completed")

def main():
    parser = argparse.ArgumentParser(
        description="OAI Archive CLI - Archive OpenAI content with style",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
  python oai_cli.py archive images     # Archive images only
  python oai_cli.py archive videos     # Archive videos only  
  python oai_cli.py archive chats      # Archive chat conversations
  python oai_cli.py archive all        # Archive everything
  python oai_cli.py stats              # Show archive statistics
        """
    )
    
    subparsers = parser.add_subparsers(dest='command', help='Available commands')
    
    # Archive command
    archive_parser = subparsers.add_parser('archive', help='Archive content')
    archive_parser.add_argument(
        'type', 
        choices=['images', 'videos', 'chats', 'all'], 
        help='Type of content to archive'
    )
    
    # Stats command
    stats_parser = subparsers.add_parser('stats', help='Show archive statistics')
    
    # Parse arguments
    args = parser.parse_args()
    
    if not args.command:
        parser.print_help()
        sys.exit(1)
    
    # Initialize CLI
    cli = OAICLI()
    if not cli.initialize():
        logger.error("Failed to initialize CLI")
        sys.exit(1)
    
    try:
        if args.command == 'archive':
            if args.type == 'images':
                result = cli.archive_images()
            elif args.type == 'videos':
                result = cli.archive_videos()
            elif args.type == 'chats':
                result = cli.archive_chats()
            elif args.type == 'all':
                result = cli.archive_all()
            
            if result['success']:
                logger.info("✅ Archive completed successfully")
                if 'results' in result:
                    # Full archive results
                    for content_type, content_result in result['results'].items():
                        if content_result['success']:
                            logger.info(f"✅ {content_type.capitalize()}: {content_result.get('downloaded', 0)} items")
                        else:
                            logger.warning(f"⚠️  {content_type.capitalize()}: {content_result.get('message', 'Failed')}")
                else:
                    # Single archive results
                    logger.info(f"✅ Processed {result.get('downloaded', 0)} items")
            else:
                logger.error(f"❌ Archive failed: {result.get('message', 'Unknown error')}")
                sys.exit(1)
        
        elif args.command == 'stats':
            result = cli.get_stats()
            if result['success']:
                stats = result['stats']
                print("\n📊 Archive Statistics")
                print("=" * 40)
                print(f"Total Images: {stats['total_images']}")
                print(f"Total Videos: {stats['total_videos']}")
                print(f"Total Chats: {stats['total_chats']}")
                print(f"Recent Images (24h): {stats['recent_images']}")
                print(f"Recent Videos (24h): {stats['recent_videos']}")
                print(f"Recent Chats (24h): {stats['recent_chats']}")
                if stats.get('total_file_size'):
                    size_mb = stats['total_file_size'] / (1024 * 1024)
                    print(f"Total Archive Size: {size_mb:.1f} MB")
                print("=" * 40)
            else:
                logger.error(f"❌ Failed to get stats: {result.get('message', 'Unknown error')}")
                sys.exit(1)
    
    except KeyboardInterrupt:
        logger.info("🛑 Archive interrupted by user")
        sys.exit(1)
    except Exception as e:
        logger.error(f"❌ Unexpected error: {e}")
        sys.exit(1)
    finally:
        cli.cleanup()

if __name__ == "__main__":
    main()
