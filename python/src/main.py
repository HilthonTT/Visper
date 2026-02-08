import sys
from pathlib import Path

# Add parent directory to Python path
root_dir = Path(__file__).parent.parent
sys.path.insert(0, str(root_dir))

import uvicorn
from src.api import router
from src.core.setup import create_application
from src.core.config import settings

def main():
    app = create_application(router=router, settings=settings)
    
    uvicorn.run(
        app,
        host="0.0.0.0",
        port=8088,
        reload=False,
        log_level="info"
    )

if __name__ == "__main__":
    main()
    