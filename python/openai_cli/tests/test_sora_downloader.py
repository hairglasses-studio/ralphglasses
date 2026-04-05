#!/usr/bin/env python3
"""
Tests for Sora downloader module
"""

import pytest
from unittest.mock import Mock, patch, MagicMock
from sora_downloader import SoraDownloader
from config import Config
from db.database import DatabaseManager


class TestSoraDownloader:
    """Test Sora downloader functionality"""
    
    @pytest.fixture
    def mock_config(self):
        """Mock configuration"""
        config = Mock(spec=Config)
        config.videos_dir = "./test_videos"
        config.max_retries = 3
        config.download_timeout = 30
        config.batch_size = 10
        config.chrome_headless = True
        config.chrome_disable_gpu = True
        config.chrome_window_size = "1920,1080"
        return config
    
    @pytest.fixture
    def mock_db(self):
        """Mock database manager"""
        db = Mock(spec=DatabaseManager)
        db.insert_video_metadata.return_value = True
        return db
    
    @pytest.fixture
    def sora_downloader(self, mock_config, mock_db):
        """Create Sora downloader instance"""
        with patch('os.makedirs'):
            return SoraDownloader(mock_config, mock_db)
    
    def test_sora_downloader_init(self, mock_config, mock_db):
        """Test Sora downloader initialization"""
        with patch('os.makedirs') as mock_makedirs:
            downloader = SoraDownloader(mock_config, mock_db)
            
            assert downloader.config == mock_config
            assert downloader.db == mock_db
            assert downloader.driver is None
            mock_makedirs.assert_called_once_with(mock_config.videos_dir, exist_ok=True)
    
    def test_setup_browser(self, sora_downloader):
        """Test browser setup"""
        with patch('selenium.webdriver.Chrome') as mock_chrome:
            mock_driver = Mock()
            mock_chrome.return_value = mock_driver
            
            driver = sora_downloader.setup_browser()
            
            assert driver == mock_driver
            mock_chrome.assert_called_once()
            mock_driver.execute_script.assert_called_once()
    
    def test_extract_prompt_text(self, sora_downloader):
        """Test prompt text extraction"""
        mock_video = Mock()
        mock_parent = Mock()
        mock_text_element = Mock()
        mock_text_element.text = "A beautiful sunset over the ocean"
        
        mock_video.find_element.return_value = mock_parent
        mock_parent.find_elements.return_value = [mock_text_element]
        
        prompt = sora_downloader._extract_prompt_text(mock_video)
        
        assert prompt == "A beautiful sunset over the ocean"
    
    def test_extract_prompt_text_no_text(self, sora_downloader):
        """Test prompt text extraction when no text is found"""
        mock_video = Mock()
        mock_parent = Mock()
        
        mock_video.find_element.return_value = mock_parent
        mock_parent.find_elements.return_value = []
        mock_video.get_attribute.return_value = None
        
        prompt = sora_downloader._extract_prompt_text(mock_video)
        
        assert prompt == ""
    
    def test_download_video_with_retry_success(self, sora_downloader):
        """Test successful video download"""
        video_data = {
            'url': 'https://example.com/video.mp4',
            'filename': 'test_video.mp4'
        }
        
        with patch('os.path.exists', return_value=False), \
             patch('requests.get') as mock_get, \
             patch('builtins.open', create=True) as mock_open:
            
            mock_response = Mock()
            mock_response.raise_for_status.return_value = None
            mock_response.iter_content.return_value = [b'video_data']
            mock_get.return_value = mock_response
            
            result = sora_downloader.download_video_with_retry(video_data)
            
            assert result is True
            mock_get.assert_called_once()
    
    def test_download_video_with_retry_file_exists(self, sora_downloader):
        """Test video download when file already exists"""
        video_data = {
            'url': 'https://example.com/video.mp4',
            'filename': 'test_video.mp4'
        }
        
        with patch('os.path.exists', return_value=True):
            result = sora_downloader.download_video_with_retry(video_data)
            
            assert result is True
    
    def test_extract_video_metadata_with_opencv(self, sora_downloader):
        """Test video metadata extraction with OpenCV"""
        with patch('cv2.VideoCapture') as mock_cv2:
            mock_cap = Mock()
            mock_cap.get.side_effect = [30.0, 900, 1920, 1080]  # fps, frame_count, width, height
            mock_cv2.return_value = mock_cap
            
            metadata = sora_downloader._extract_video_metadata('/path/to/video.mp4')
            
            assert metadata['fps'] == 30.0
            assert metadata['frame_count'] == 900
            assert metadata['width'] == 1920
            assert metadata['height'] == 1080
            assert metadata['resolution'] == '1920x1080'
            mock_cap.release.assert_called_once()
    
    def test_extract_video_metadata_without_opencv(self, sora_downloader):
        """Test video metadata extraction without OpenCV"""
        with patch('builtins.__import__', side_effect=ImportError("No module named 'cv2'")):
            metadata = sora_downloader._extract_video_metadata('/path/to/video.mp4')
            
            assert metadata == {}
    
    def test_archive_videos_no_videos_found(self, sora_downloader):
        """Test archive videos when no videos are found"""
        with patch.object(sora_downloader, 'login_and_scrape_videos', return_value=[]):
            result = sora_downloader.archive_videos()
            
            assert result['success'] is False
            assert result['message'] == 'No videos found'
    
    def test_archive_videos_success(self, sora_downloader):
        """Test successful video archiving"""
        video_data = {
            'video_id': 'test_id',
            'url': 'https://example.com/video.mp4',
            'filename': 'test_video.mp4',
            'title': 'Test Video',
            'duration': 30.0,
            'resolution': '1920x1080',
            'prompt_text': 'A beautiful sunset'
        }
        
        with patch.object(sora_downloader, 'login_and_scrape_videos', return_value=[video_data]), \
             patch.object(sora_downloader, 'download_video_with_retry', return_value=True), \
             patch('os.path.getsize', return_value=1024), \
             patch.object(sora_downloader, '_extract_video_metadata', return_value={}):
            
            result = sora_downloader.archive_videos()
            
            assert result['success'] is True
            assert result['total_found'] == 1
            assert result['downloaded'] == 1
            assert result['metadata_inserted'] == 1
    
    def test_close(self, sora_downloader):
        """Test cleanup method"""
        mock_driver = Mock()
        sora_downloader.driver = mock_driver
        
        sora_downloader.close()
        
        mock_driver.quit.assert_called_once()
    
    def test_close_no_driver(self, sora_downloader):
        """Test cleanup method when no driver exists"""
        sora_downloader.driver = None
        
        # Should not raise an exception
        sora_downloader.close() 