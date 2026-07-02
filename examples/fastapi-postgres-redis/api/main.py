import os

from fastapi import FastAPI

app = FastAPI(title="Macpose FastAPI Example")


def service_host(name: str) -> str:
    key = f"MACPOSE_SERVICE_{name.upper()}_HOST"
    return os.getenv(key, name)


@app.get("/health")
def health():
    db_host = service_host("db")
    redis_host = service_host("redis")
    return {
        "status": "ok",
        "database_host": db_host,
        "redis_host": redis_host,
    }


@app.get("/")
def root():
    return {"message": "Hello from Macpose + FastAPI"}
