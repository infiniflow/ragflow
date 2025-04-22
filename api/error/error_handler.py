from flask import Flask


def init_app(app: Flask):

    @app.errorhandler(ValueError)
    def handle_value_error(error):
        return {"message": str(error), "code": 400}, 400

    @app.errorhandler(Exception)
    def handle_value_error(error):
        return {"message": str(error), "code": 500}, 500
