
import requests
from flask import Flask
from flask import request


app = Flask(__name__)


@app.route("/")
def index():
    return { "status": "ok" }


@app.route("/rates")
def getQuote():
    # Declare string with the URL
    url = "https://api.fiscaldata.treasury.gov/services/api/fiscal_service/v2/accounting/od/avg_interest_rates?page[number]=1&page[size]=10"
    
    headers = {
        "Content-Type": "application/json",
    }

    # Fetch the data
    response = requests.get(url, headers=headers)

    if (response.status_code != 200):
        return { "status": "error" }
    
    # Convert the response to JSON
    data = response.json()

    # Return the data
    return data
    

if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')
