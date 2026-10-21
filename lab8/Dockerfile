FROM python:3.12-slim
WORKDIR /app
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt
COPY app ./app
COPY data ./data
COPY skills ./skills
RUN mkdir /outputs && useradd --uid 10001 --create-home agent && chown agent:agent /outputs
USER agent
ENV PYTHONUNBUFFERED=1 OUTPUT_DIR=/outputs
EXPOSE 8000 8001 8002
CMD ["uvicorn", "app.agent:app", "--host", "0.0.0.0", "--port", "8000"]
