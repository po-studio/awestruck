import sys
from flask import Flask, render_template, request
from pythonosc import udp_client

client = None

app = Flask(__name__)

def send_osc_message(address, value):
    global client
    if client is None:
        try:
            client = udp_client.SimpleUDPClient('worker', 57120)
        except Exception:
            # Could not establish connection
            log("Could not establish osc connection")
            return False
    log(f"Sending osc message! {address} {value}")
    client.send_message(f'/{address}', float(value))
    return True
    
def log(message):
    print(message)
    sys.stdout.flush()

@app.route('/send_osc')
def send_osc():
    # Endpoint to receive OSC message command via HTTP
    # Will forward the messages to the worker service
    address = request.args.get('address')
    value = request.args.get('value')
    result = send_osc_message(address, value)
    return {'result': result}
