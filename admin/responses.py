from flask import jsonify

def success_response(data=None, message="Success", code = 0):
    return jsonify({
        "code": code,
        "message": message,
        "data": data
    }), 200

def error_response(message="Error", code=-1, data=None):
    return jsonify({
        "code": code,
        "message": message,
        "data": data
    }), 400