def sanitize_path(path: str) -> str:
    return path.strip("/").replace("/", "_")
