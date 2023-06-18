import os
import numpy as np
from flask import Flask, jsonify, request
from implicit.als import AlternatingLeastSquares
from implicit.datasets.lastfm import get_lastfm
from implicit.nearest_neighbours import bm25_weight


app = Flask(__name__)

model = AlternatingLeastSquares(factors=50, dtype=np.float32)


try:
    artists = np.load("artists.npy", allow_pickle=True)
    model = model.load("model.npz")
except Exception as e:
    print(e)
    # Load lastfm dataset and fit model
    artists, _, plays = get_lastfm()
    plays = bm25_weight(plays, K1=100, B=0.8).tocsr()
    user_plays = plays.T.tocsr()
    model.fit(user_plays[:2000])
    model.save("model")
    np.save("artists", artists)


@app.route('/recommend', methods=['POST'])
def recommend():
    # Parse JSON request body
    req_data = request.form
    artist_name = req_data['artist']

    # Get artist ID
    artist_id = None
    for i, a in enumerate(artists):
        if a.lower() == artist_name.lower():
            artist_id = i
            break

    if artist_id is None:
        return jsonify({'error': 'Artist not found'})

    # Get recommended artists
    similar_ids, _ = model.similar_items(artist_id, N=10)
    similar_artists = [artists[i] for i in similar_ids]

    return jsonify({'similar_artists': similar_artists})


@app.route("/")
def index():
    return  '''
<!DOCTYPE html>
<html>
    <head>
        <link rel="stylesheet" href="https://maxcdn.bootstrapcdn.com/bootstrap/4.5.2/css/bootstrap.min.css">
    </head>
    <body style="display: flex; justify-content: center; align-items: center; height: 100vh; background-color: #f8f9fa;">
        <div style="width: 40%; text-align: center;">
            <h1 style="color: #4a4a4a; margin-bottom: 30px;">Recommendation API</h1>
            <form action="/recommend" method="POST" style="display: flex; flex-direction: column; align-items: center;">
                <label for="artist" style="color: #4a4a4a; margin-bottom: 10px;">Artist name:</label>
                <input type="text" id="artist" name="artist" required autofocus class="form-control" style="margin-bottom: 15px;">
                <input type="submit" value="Get recommendations" class="btn btn-primary">
            </form>
        </div>
    </body>
</html>'''


if os.getenv("DEFANG_FQDN"):
    print("FQDN:", "https://"+os.getenv("DEFANG_FQDN"))


if __name__ == '__main__':
    app.run(debug=True, host='0.0.0.0')
