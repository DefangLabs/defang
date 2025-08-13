# app.py
# Samaki Shop Profit & Loss – Role-based Flask app
# Single-file app with SQLite, basic auth, employee/admin roles, and CSV export.

from flask import Flask, request, redirect, url_for, session, send_file, abort
from flask import render_template_string, flash
from werkzeug.security import generate_password_hash, check_password_hash
import sqlite3
from contextlib import closing
from datetime import datetime
import io
import csv

app = Flask(__name__)
app.config['SECRET_KEY'] = 'change-this-in-production'
app.config['DATABASE'] = 'samaki.db'

############################
# Database helpers
############################

def get_db():
    conn = sqlite3.connect(app.config['DATABASE'])
    conn.row_factory = sqlite3.Row
    return conn

SCHEMA_SQL = """
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    username TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK(role IN ('admin','employee'))
);

CREATE TABLE IF NOT EXISTS settings (
    id INTEGER PRIMARY KEY CHECK (id = 1),
    buying_price_per_kg REAL NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS entries (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    date TEXT NOT NULL,
    kgs_bought REAL,
    kgs_sold REAL,
    total_kgs_remaining REAL,
    buying_price_per_kg REAL,
    total_buying_cost REAL,
    selling_price_per_kg REAL,
    total_selling_revenue REAL,
    transport_fee REAL,
    profit_loss REAL,
    created_by INTEGER,
    FOREIGN KEY(created_by) REFERENCES users(id)
);
"""

def init_db():
    with closing(get_db()) as db:
        db.executescript(SCHEMA_SQL)
        # Seed settings (single row)
        cur = db.execute('SELECT COUNT(*) as c FROM settings WHERE id = 1')
        if cur.fetchone()['c'] == 0:
            db.execute('INSERT INTO settings (id, buying_price_per_kg) VALUES (1, ?)', (310.0,))
        # Seed default admin if none exists
        cur = db.execute('SELECT COUNT(*) as c FROM users')
        if cur.fetchone()['c'] == 0:
            db.execute('INSERT INTO users (username, password_hash, role) VALUES (?,?,?)', (
                'admin', generate_password_hash('admin123'), 'admin'
            ))
        db.commit()

@app.before_first_request
def setup():
    init_db()

############################
# Auth utilities
############################

def current_user():
    uid = session.get('user_id')
    if not uid:
        return None
    db = get_db()
    user = db.execute('SELECT * FROM users WHERE id = ?', (uid,)).fetchone()
    return user

def login_required(role=None):
    def decorator(fn):
        def wrapper(*args, **kwargs):
            user = current_user()
            if not user:
                return redirect(url_for('login', next=request.path))
            if role and user['role'] != role:
                abort(403)
            return fn(*args, **kwargs)
        wrapper.__name__ = fn.__name__
        return wrapper
    return decorator

############################
# Routes: Auth
############################

@app.route('/')
def index():
    user = current_user()
    if not user:
        return redirect(url_for('login'))
    if user['role'] == 'admin':
        return redirect(url_for('admin_dashboard'))
    return redirect(url_for('employee_entry'))

@app.route('/login', methods=['GET','POST'])
def login():
    if request.method == 'POST':
        username = request.form.get('username','').strip()
        password = request.form.get('password','')
        db = get_db()
        user = db.execute('SELECT * FROM users WHERE username = ?', (username,)).fetchone()
        if user and check_password_hash(user['password_hash'], password):
            session['user_id'] = user['id']
            flash('Logged in successfully.', 'success')
            if user['role'] == 'admin':
                return redirect(url_for('admin_dashboard'))
            return redirect(url_for('employee_entry'))
        flash('Invalid credentials', 'error')
    return render_template_string(TPL_LOGIN)

@app.route('/logout')
def logout():
    session.clear()
    return redirect(url_for('login'))

############################
# Routes: Admin
############################

@app.route('/admin/dashboard')
@login_required(role='admin')
def admin_dashboard():
    db = get_db()
    rows = db.execute('SELECT e.*, u.username as created_by_username FROM entries e LEFT JOIN users u ON e.created_by = u.id ORDER BY date DESC, id DESC').fetchall()
    settings = db.execute('SELECT * FROM settings WHERE id = 1').fetchone()
    return render_template_string(TPL_ADMIN_DASH, rows=rows, settings=settings)

@app.route('/admin/settings', methods=['POST'])
@login_required(role='admin')
def admin_update_settings():
    buying = request.form.get('buying_price_per_kg')
    try:
        buying = float(buying)
    except (TypeError, ValueError):
        flash('Buying price must be a number', 'error')
        return redirect(url_for('admin_dashboard'))
    db = get_db()
    db.execute('UPDATE settings SET buying_price_per_kg = ? WHERE id = 1', (buying,))
    db.commit()
    flash('Settings updated.', 'success')
    return redirect(url_for('admin_dashboard'))

@app.route('/admin/create_user', methods=['POST'])
@login_required(role='admin')
def admin_create_user():
    username = request.form.get('username','').strip()
    password = request.form.get('password','')
    role = request.form.get('role','employee')
    if role not in ('admin','employee'):
        role = 'employee'
    if not username or not password:
        flash('Username and password required.', 'error')
        return redirect(url_for('admin_dashboard'))
    try:
        db = get_db()
        db.execute('INSERT INTO users (username, password_hash, role) VALUES (?,?,?)', (username, generate_password_hash(password), role))
        db.commit()
        flash('User created.', 'success')
    except sqlite3.IntegrityError:
        flash('Username already exists.', 'error')
    return redirect(url_for('admin_dashboard'))

@app.route('/export.csv')
@login_required(role='admin')
def export_csv():
    db = get_db()
    rows = db.execute('SELECT date, kgs_bought, kgs_sold, total_kgs_remaining, buying_price_per_kg, total_buying_cost, selling_price_per_kg, total_selling_revenue, transport_fee, profit_loss FROM entries ORDER BY date ASC, id ASC').fetchall()
    # Stream CSV
    output = io.StringIO()
    writer = csv.writer(output)
    writer.writerow(['Date','Kgs Bought','Kgs Sold Today','Total Kgs Remaining','Buying Price per Kg','Total Buying Cost','Selling Price per Kg','Total Selling Revenue','Transport Fee','Profit/Loss'])
    for r in rows:
        writer.writerow([r['date'], r['kgs_bought'], r['kgs_sold'], r['total_kgs_remaining'], r['buying_price_per_kg'], r['total_buying_cost'], r['selling_price_per_kg'], r['total_selling_revenue'], r['transport_fee'], r['profit_loss']])
    mem = io.BytesIO()
    mem.write(output.getvalue().encode('utf-8'))
    mem.seek(0)
    return send_file(mem, mimetype='text/csv', as_attachment=True, download_name='samaki_entries.csv')

############################
# Routes: Employee
############################

@app.route('/employee/entry', methods=['GET','POST'])
@login_required()
def employee_entry():
    user = current_user()
    db = get_db()
    settings = db.execute('SELECT * FROM settings WHERE id = 1').fetchone()

    if request.method == 'POST':
        # Allowed fields for employees
        date_str = request.form.get('date') or datetime.now().strftime('%Y-%m-%d')
        kgs_bought = request.form.get('kgs_bought') or 0
        kgs_sold = request.form.get('kgs_sold') or 0
        total_kgs_remaining = request.form.get('total_kgs_remaining') or 0
        selling_price_per_kg = request.form.get('selling_price_per_kg') or 0
        transport_fee = request.form.get('transport_fee') or 0

        # Convert to numbers safely
        def f2(x):
            try:
                return float(x)
            except (TypeError, ValueError):
                return 0.0
        kgs_bought = f2(kgs_bought)
        kgs_sold = f2(kgs_sold)
        total_kgs_remaining = f2(total_kgs_remaining)
        selling_price_per_kg = f2(selling_price_per_kg)
        transport_fee = f2(transport_fee)

        # Hidden/calculated fields (employee cannot set)
        buying_price_per_kg = float(settings['buying_price_per_kg'] or 0)
        total_buying_cost = kgs_bought * buying_price_per_kg
        total_selling_revenue = kgs_sold * selling_price_per_kg
        profit_loss = total_selling_revenue - (total_buying_cost + transport_fee)

        db.execute('''
            INSERT INTO entries (date, kgs_bought, kgs_sold, total_kgs_remaining, buying_price_per_kg, total_buying_cost, selling_price_per_kg, total_selling_revenue, transport_fee, profit_loss, created_by)
            VALUES (?,?,?,?,?,?,?,?,?,?,?)
        ''', (date_str, kgs_bought, kgs_sold, total_kgs_remaining, buying_price_per_kg, total_buying_cost, selling_price_per_kg, total_selling_revenue, transport_fee, profit_loss, user['id']))
        db.commit()
        flash('Entry saved.', 'success')
        return redirect(url_for('employee_entry'))

    # Show last 10 of the employee's own entries (only allowed fields + date)
    recent = db.execute('''
        SELECT date, kgs_bought, kgs_sold, total_kgs_remaining, selling_price_per_kg, transport_fee
        FROM entries WHERE created_by = ? ORDER BY date DESC, id DESC LIMIT 10
    ''', (user['id'],)).fetchall()
    return render_template_string(TPL_EMPLOYEE_FORM, recent=recent, settings=settings)

############################
# Templates (inline for single-file app)
############################

BASE_CSS = """
<style>
body{font-family: system-ui, -apple-system, Segoe UI, Roboto, Ubuntu, Cantarell, Noto Sans, Helvetica, Arial, "Apple Color Emoji", "Segoe UI Emoji"; margin:0; background:#f7f7fb;}
.container{max-width:1000px;margin:0 auto;padding:24px;}
.card{background:#fff;border-radius:16px;box-shadow:0 10px 25px rgba(0,0,0,.06);padding:24px;margin:16px 0;}
.input{width:100%;padding:12px 14px;border:1px solid #e5e7eb;border-radius:12px;}
.label{font-weight:600;margin-bottom:6px;display:block;}
.row{display:grid;grid-template-columns:repeat(2,1fr);gap:16px;}
.actions{display:flex;gap:12px;}
button{padding:12px 16px;border:none;border-radius:12px;background:#111827;color:#fff;font-weight:600;cursor:pointer}
button.secondary{background:#e5e7eb;color:#111827}
.nav{display:flex;gap:12px;align-items:center;justify-content:space-between}
.badge{display:inline-block;background:#eef2ff;color:#4338ca;border-radius:999px;padding:6px 10px;font-weight:700;font-size:12px}
.table{width:100%;border-collapse:collapse}
.table th,.table td{padding:10px;border-bottom:1px solid #eee;text-align:left;font-size:14px}
.flash{padding:10px 12px;border-radius:10px;margin-bottom:10px}
.flash.success{background:#ecfdf5;color:#065f46}
.flash.error{background:#fef2f2;color:#991b1b}
.small{font-size:12px;color:#6b7280}
.hidden{display:none}
</style>
"""

TPL_LOGIN = """
<!doctype html>
<title>Login · Samaki Shop</title>
""" + BASE_CSS + """
<div class="container">
  <div class="card">
    <div class="nav">
      <h2>Samaki Shop · Login</h2>
      <span class="badge">Default admin: admin / admin123</span>
    </div>
    {% with messages = get_flashed_messages(with_categories=true) %}
      {% if messages %}
        {% for cat,msg in messages %}
          <div class="flash {{cat}}">{{msg}}</div>
        {% endfor %}
      {% endif %}
    {% endwith %}
    <form method="post">
      <div class="row">
        <div>
          <label class="label">Username</label>
          <input class="input" name="username" required>
        </div>
        <div>
          <label class="label">Password</label>
          <input class="input" type="password" name="password" required>
        </div>
      </div>
      <div class="actions" style="margin-top:16px">
        <button type="submit">Sign In</button>
      </div>
      <p class="small">Tip: Login as admin, go to Dashboard → Create User to add employees.</p>
    </form>
  </div>
</div>
"""

TPL_EMPLOYEE_FORM = """
<!doctype html>
<title>Employee Entry · Samaki Shop</title>
""" + BASE_CSS + """
<div class="container">
  <div class="nav">
    <h2>Employee · Daily Entry</h2>
    <div>
      <a href="{{ url_for('logout') }}"><button class="secondary">Logout</button></a>
    </div>
  </div>
  <div class="card">
    {% with messages = get_flashed_messages(with_categories=true) %}
      {% if messages %}
        {% for cat,msg in messages %}
          <div class="flash {{cat}}">{{msg}}</div>
        {% endfor %}
      {% endif %}
    {% endwith %}

    <form method="post">
      <div class="row">
        <div>
          <label class="label">Date</label>
          <input class="input" type="date" name="date" value="{{ ("now"|date('Y-m-d')) }}" required>
        </div>
        <div>
          <label class="label">Kgs Bought</label>
          <input class="input" type="number" step="0.01" name="kgs_bought" placeholder="e.g. 10">
        </div>
        <div>
          <label class="label">Kgs Sold Today</label>
          <input class="input" type="number" step="0.01" name="kgs_sold" placeholder="e.g. 8">
        </div>
        <div>
          <label class="label">Total Kgs Remaining</label>
          <input class="input" type="number" step="0.01" name="total_kgs_remaining" placeholder="e.g. 2">
        </div>
        <div>
          <label class="label">Selling Price per Kg</label>
          <input class="input" type="number" step="0.01" name="selling_price_per_kg" placeholder="e.g. 410">
        </div>
        <div>
          <label class="label">Transport Fee</label>
          <input class="input" type="number" step="0.01" name="transport_fee" placeholder="e.g. 200">
        </div>
      </div>
      <div class="actions" style="margin-top:16px">
        <button type="submit">Save Entry</button>
      </div>
      <p class="small">Note: Buying price is set by Admin (currently {{ settings.buying_price_per_kg }} per Kg) and hidden from employees.</p>
    </form>
  </div>

  <div class="card">
    <h3>Your Recent Entries (limited view)</h3>
    <table class="table">
      <thead>
        <tr>
          <th>Date</th>
          <th>Kgs Bought</th>
          <th>Kgs Sold</th>
          <th>Total Kgs Remaining</th>
          <th>Selling Price/Kg</th>
          <th>Transport Fee</th>
        </tr>
      </thead>
      <tbody>
        {% for r in recent %}
        <tr>
          <td>{{ r['date'] }}</td>
          <td>{{ r['kgs_bought'] or '' }}</td>
          <td>{{ r['kgs_sold'] or '' }}</td>
          <td>{{ r['total_kgs_remaining'] or '' }}</td>
          <td>{{ r['selling_price_per_kg'] or '' }}</td>
          <td>{{ r['transport_fee'] or '' }}</td>
        </tr>
        {% else %}
        <tr><td colspan="6" class="small">No entries yet.</td></tr>
        {% endfor %}
      </tbody>
    </table>
  </div>
</div>
"""

TPL_ADMIN_DASH = """
<!doctype html>
<title>Admin Dashboard · Samaki Shop</title>
""" + BASE_CSS + """
<div class="container">
  <div class="nav">
    <h2>Admin · Dashboard</h2>
    <div class="actions">
      <a href="{{ url_for('logout') }}"><button class="secondary">Logout</button></a>
      <a href="{{ url_for('export_csv') }}"><button>Export CSV</button></a>
    </div>
  </div>

  {% with messages = get_flashed_messages(with_categories=true) %}
    {% if messages %}
      {% for cat,msg in messages %}
        <div class="flash {{cat}}">{{msg}}</div>
      {% endfor %}
    {% endif %}
  {% endwith %}

  <div class="card">
    <h3>Settings</h3>
    <form method="post" action="{{ url_for('admin_update_settings') }}">
      <label class="label">Buying Price per Kg</label>
      <div class="row">
        <div>
          <input class="input" type="number" step="0.01" name="buying_price_per_kg" value="{{ settings.buying_price_per_kg }}" required>
        </div>
        <div>
          <button type="submit">Save</button>
        </div>
      </div>
      <p class="small">This value is hidden from employees and used to calculate Total Buying Cost and Profit/Loss.</p>
    </form>
  </div>

  <div class="card">
    <h3>Create User</h3>
    <form method="post" action="{{ url_for('admin_create_user') }}">
      <div class="row">
        <div>
          <label class="label">Username</label>
          <input class="input" name="username" required>
        </div>
        <div>
          <label class="label">Password</label>
          <input class="input" type="password" name="password" required>
        </div>
        <div>
          <label class="label">Role</label>
          <select class="input" name="role">
            <option value="employee">Employee</option>
            <option value="admin">Admin</option>
          </select>
        </div>
      </div>
      <div class="actions" style="margin-top:16px">
        <button type="submit">Add User</button>
      </div>
    </form>
  </div>

  <div class="card">
    <h3>All Entries (full view)</h3>
    <table class="table">
      <thead>
        <tr>
          <th>Date</th>
          <th>Kgs Bought</th>
          <th>Kgs Sold</th>
          <th>Total Kgs Remaining</th>
          <th>Buying Price/Kg</th>
          <th>Total Buying Cost</th>
          <th>Selling Price/Kg</th>
          <th>Total Selling Revenue</th>
          <th>Transport Fee</th>
          <th>Profit/Loss</th>
          <th>By</th>
        </tr>
      </thead>
      <tbody>
        {% for r in rows %}
        <tr>
          <td>{{ r['date'] }}</td>
          <td>{{ r['kgs_bought'] or '' }}</td>
          <td>{{ r['kgs_sold'] or '' }}</td>
          <td>{{ r['total_kgs_remaining'] or '' }}</td>
          <td>{{ r['buying_price_per_kg'] or '' }}</td>
          <td>{{ r['total_buying_cost'] or '' }}</td>
          <td>{{ r['selling_price_per_kg'] or '' }}</td>
          <td>{{ r['total_selling_revenue'] or '' }}</td>
          <td>{{ r['transport_fee'] or '' }}</td>
          <td>{{ r['profit_loss'] or '' }}</td>
          <td>{{ r['created_by_username'] or '' }}</td>
        </tr>
        {% else %}
        <tr><td colspan="11" class="small">No entries yet.</td></tr>
        {% endfor %}
      </tbody>
    </table>
  </div>
</div>
"""

if __name__ == '__main__':
    app.run(debug=True)
