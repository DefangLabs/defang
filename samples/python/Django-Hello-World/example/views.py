# example/views.py
from datetime import datetime

from django.http import HttpResponse

def index(request):
    now = datetime.now()
    html = f'''
    <html>
        <body>
            <h1>Defang says: Hello World!</h1>
            <p>The current time is { now }.</p>
        </body>
    </html>
    '''
    return HttpResponse(html)