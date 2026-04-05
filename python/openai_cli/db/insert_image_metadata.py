
import psycopg2
import os

def insert_image_metadata(image_id, filename, prompt_hash, created_at, tags):
    conn = psycopg2.connect(os.getenv("PG_CONN_STR"))
    cur = conn.cursor()
    cur.execute("""
        INSERT INTO images (image_id, filename, prompt_hash, created_at, tags, archived, archived_at)
        VALUES (%s, %s, %s, %s, %s, TRUE, NOW())
        ON CONFLICT (image_id) DO NOTHING
    """, (image_id, filename, prompt_hash, created_at, tags))
    conn.commit()
    cur.close()
    conn.close()
