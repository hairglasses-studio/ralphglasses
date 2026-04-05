
from flask import Flask, render_template, jsonify, request, redirect, url_for
from flask_cors import CORS
import logging
from datetime import datetime, timedelta
import os
from config import Config
from db.database import DatabaseManager
from typing import Optional

# Setup logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

app = Flask(__name__)
CORS(app)  # Enable CORS for API endpoints

# Global config and db manager
config: Optional[Config] = None
db: Optional[DatabaseManager] = None

def init_app():
    """Initialize the Flask app with configuration"""
    global config, db
    
    config = Config.from_env()
    config.validate()
    
    db = DatabaseManager(config)
    logger.info("Web app initialized successfully")

@app.route("/")
def index():
    """Main dashboard page"""
    if db is None:
        return render_template('error.html', error="Database not initialized")
        
    try:
        # Get archive statistics
        stats = db.get_archive_stats()
        
        # Get recent content
        recent_images = db.get_recent_images(10)
        recent_videos = db.get_recent_videos(10)
        recent_chats = db.get_recent_chats(10)
        
        return render_template('dashboard.html', 
                            stats=stats,
                            recent_images=recent_images,
                            recent_videos=recent_videos,
                            recent_chats=recent_chats)
    except Exception as e:
        logger.error(f"Error loading dashboard: {e}")
        return render_template('error.html', error=str(e))

@app.route("/api/stats")
def api_stats():
    """API endpoint for archive statistics"""
    if db is None:
        return jsonify({"error": "Database not initialized"}), 500
        
    try:
        stats = db.get_archive_stats()
        return jsonify(stats)
    except Exception as e:
        logger.error(f"Error getting stats: {e}")
        return jsonify({"error": str(e)}), 500

@app.route("/api/images")
def api_images():
    """API endpoint for recent images"""
    if db is None:
        return jsonify({"error": "Database not initialized"}), 500
        
    try:
        limit = request.args.get('limit', 20, type=int)
        images = db.get_recent_images(limit)
        return jsonify(images)
    except Exception as e:
        logger.error(f"Error getting images: {e}")
        return jsonify({"error": str(e)}), 500

@app.route("/api/videos")
def api_videos():
    """API endpoint for recent videos"""
    if db is None:
        return jsonify({"error": "Database not initialized"}), 500
        
    try:
        limit = request.args.get('limit', 20, type=int)
        videos = db.get_recent_videos(limit)
        return jsonify(videos)
    except Exception as e:
        logger.error(f"Error getting videos: {e}")
        return jsonify({"error": str(e)}), 500

@app.route("/api/chats")
def api_chats():
    """API endpoint for recent chat conversations"""
    if db is None:
        return jsonify({"error": "Database not initialized"}), 500
        
    try:
        limit = request.args.get('limit', 20, type=int)
        chats = db.get_recent_chats(limit)
        return jsonify(chats)
    except Exception as e:
        logger.error(f"Error getting chats: {e}")
        return jsonify({"error": str(e)}), 500

@app.route("/api/chats/<conversation_id>/messages")
def api_chat_messages(conversation_id):
    """API endpoint for chat messages in a conversation"""
    if db is None:
        return jsonify({"error": "Database not initialized"}), 500
        
    try:
        limit = request.args.get('limit', 100, type=int)
        messages = db.get_chat_messages(conversation_id, limit)
        return jsonify(messages)
    except Exception as e:
        logger.error(f"Error getting chat messages: {e}")
        return jsonify({"error": str(e)}), 500

@app.route("/api/search")
def api_search():
    """API endpoint for searching content"""
    if db is None:
        return jsonify({"error": "Database not initialized"}), 500
        
    try:
        query = request.args.get('q', '')
        content_type = request.args.get('type', 'all')  # images, videos, chats, all
        limit = request.args.get('limit', 50, type=int)
        
        if not query:
            return jsonify({"error": "Query parameter required"}), 400
        
        # Use the enhanced search method
        results = db.search_content(query, content_type, limit)
        return jsonify(results)
        
    except Exception as e:
        logger.error(f"Error searching: {e}")
        return jsonify({"error": str(e)}), 500

@app.route("/health")
def health_check():
    """Health check endpoint"""
    if db is None:
        return jsonify({
            "status": "unhealthy",
            "timestamp": datetime.now().isoformat(),
            "error": "Database not initialized"
        }), 500
        
    try:
        # Test database connection
        db.execute_query("SELECT 1")
        return jsonify({
            "status": "healthy",
            "timestamp": datetime.now().isoformat(),
            "database": "connected"
        })
    except Exception as e:
        logger.error(f"Health check failed: {e}")
        return jsonify({
            "status": "unhealthy",
            "timestamp": datetime.now().isoformat(),
            "error": str(e)
        }), 500

@app.errorhandler(404)
def not_found(error):
    return render_template('404.html'), 404

@app.errorhandler(500)
def internal_error(error):
    return render_template('error.html', error="Internal server error"), 500

if __name__ == "__main__":
    init_app()
    if config is None:
        logger.error("Failed to initialize configuration")
        exit(1)
    app.run(host=config.web_host, port=config.web_port, debug=config.web_debug)
