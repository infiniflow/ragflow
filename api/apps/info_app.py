@manager.route('/health', methods=['GET'])
def health():
    return "ok"