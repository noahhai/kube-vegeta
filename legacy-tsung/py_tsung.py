import subprocess
import os
import shutil
from SimpleHTTPServer import SimpleHTTPRequestHandler
import SocketServer
import click
import re
import jwt
import datetime
import requests
from time import sleep
import json
import time
import random
from xml.dom.minidom import Text, Element



def get_token(username):
    payload = {
        "UserName": username,
        'exp': datetime.datetime.utcnow() + datetime.timedelta(days=1)
    }
    symmetric_secret = 'my-secret'

    encoded = jwt.encode(payload, symmetric_secret, algorithm='HS256')
    # encoded = jwt.encode(payload, private_key, algorithm='RS256')
    return encoded


class TsungHTTPRequestHandler(SimpleHTTPRequestHandler):
    def translate_path(self, path):
        print path
        if path == '/':
            path = '/graph.html'
        path = SimpleHTTPRequestHandler.translate_path(self, path)
        return path


def update_tsung_config(tsung_config_path, duration, arrival_rate, max_users, user_name = None, entity_id = None, api_path = None):
    with open(tsung_config_path, 'r') as f:
        content = f.read()

    content = re.sub('<load [^>]+>', '<load duration="{}" unit="second">'.format(duration), content, flags=re.M)
    content = re.sub('<arrivalphase [^>]+>',
                     '<arrivalphase phase="1" duration="{}" unit="second">'.format(duration), content, flags=re.M)
    content = re.sub('<users [^>]+>',
                     '<users arrivalrate="{}" unit="second" maxnumber="{}"/>'.format(arrival_rate, max_users), content, flags=re.M)
    content = re.sub('<client [^>]+>',
                     '<client host="localhost" maxusers="{}" use_controller_vm="true"/>'.format(max_users), content, flags=re.M)
    if user_name:
        encoded_auth_token = get_token(user_name)
        content = re.sub('<http_header name="Authorization" [^>]+>',
                     '<http_header name="Authorization" value="{}" />'.format(encoded_auth_token), content,
                     flags=re.M)
    if entity_id:
        # t = Text()
        # e = Element('')
        #
        # t.data = json.dumps({'username': user_name, 'entityid': entity_id})
        # e.appendChild(t)
        #
        # data = e.toxml()[3:-4]
        # data
        #data =  json.dumps({'username': user_name, 'entityid': entity_id}).replace('"','&quot;')
        content = re.sub('<http url=[^>]+>',
                     '<http url="/{}?username={}&amp;entityid={}" method="GET" >'.format(api_path,user_name,entity_id), content,
                     flags=re.M)

    with open(tsung_config_path, 'w') as f:
        f.write(content)


@click.command()
@click.option('--duration', default=60, help='Duration in secvisonds.')
@click.option('--arrival-rate', default=20, help='Arrival rate of users per second.')
@click.option('--max-users', default=1000, help='Maximum number of Users.')
@click.option('--profile/--no-profile', default=True, help='Maximum number of Users.')
@click.option('--reports/--no-reports', default=True, help='Maximum number of Users.')
def do_work(duration, arrival_rate, max_users, profile, reports):
    PORT = 8091

    IS_AWS_LAMBDA_PY = False
    IS_AWS_LAMBDA_GO = True
    IS_AWS_EC2_GO = not IS_AWS_LAMBDA_PY and not IS_AWS_LAMBDA_GO
    IS_AZURE_APP_SERVICE = not IS_AWS_LAMBDA_PY and not IS_AWS_EC2_GO and not IS_AWS_LAMBDA_GO
    if IS_AWS_LAMBDA_PY:
        relative_config_path = '~/.tsung/tsung_aws.xml'
        USER_NAME = 'user002'
        ENTITY_ID = 'e9256f09-838b-4110-9a33-edde4101438b'
        API_PATH = 'prod'
    elif IS_AWS_LAMBDA_GO:
        relative_config_path = '~/.tsung/tsung_aws_lambda_go.xml'
        USER_NAME = 'noah'
        ENTITY_ID = '2bdb9abd-5f89-434e-a4e4-7fdd82d71d58'
        API_PATH = 'Prod/secret'
    elif IS_AWS_EC2_GO:
        relative_config_path = '~/.tsung/tsung_aws.xml'
        USER_NAME = 'noah'
        ENTITY_ID = '2bdb9abd-5f89-434e-a4e4-7fdd82d71d58'
        API_PATH = 'api/message'
    elif IS_AZURE_APP_SERVICE:
        relative_config_path = '~/.tsung/tsung.xml'
        USER_NAME = None
        ENTITY_ID = None
        API_PATH = None

    if profile:
        tsung_config_path = os.path.expanduser(relative_config_path)

        update_tsung_config(tsung_config_path, duration, arrival_rate, max_users, USER_NAME, ENTITY_ID, API_PATH)

        print(
            'Starting Tsung with Duration={} seconds, Arrival Rate={}/sec, Max Users={}'.format(duration, arrival_rate,
                                                                                                max_users))
        otp_proc = subprocess.Popen(['tsung', '-f', tsung_config_path, 'start'], stdout=subprocess.PIPE,
                                    stderr=subprocess.PIPE)
        count = 0
        while otp_proc.poll() == None:
            # We can do other things here while we wait
            time.sleep(1)
            otp_proc.poll()
            count = count + 1
            if count % 5 == 0:
                print 'Tsung running for {} of {} seconds.'.format(count, duration)
        (results, errors) = otp_proc.communicate()
        # Erlang kind of looks like it throws an error here
        # if errors != '':
        #     raise Exception(errors)

    if reports:
        parent_output_path = os.path.expanduser('~/.tsung/log')
        run_data = os.listdir(parent_output_path)

        latest_run_folder = next(iter(sorted(run_data, reverse=True)), None)
        if not latest_run_folder:
            raise Exception('Cannot find folder of latest run')
        latest_run_folder = os.path.join(parent_output_path, latest_run_folder)
        print('Found output location from latest run: ' + latest_run_folder)

        output_location = os.path.expanduser('~/output-tsung')

        if os.path.isdir(output_location):
            shutil.rmtree(output_location)
        time.sleep(.5)
        os.makedirs(output_location, 0o777)

        os.chdir(output_location)
        subprocess.call(
            ['/usr/local/lib/tsung/bin/tsung_stats.pl', '--stats', os.path.join(latest_run_folder, 'tsung.log')])

        print('Waiting 5 seconds for beam vm to quit so can take over monitor port with reports.')
        subprocess.Popen(["kill", "-9", "$(lsof -ti tcp:{})".format(PORT)])
        subprocess.Popen(["pkill", "-f", "erlang"])
        time.sleep(5)

        httpd = SocketServer.TCPServer(("", PORT), TsungHTTPRequestHandler)

        print "Serving reports on port {}".format(PORT)
        httpd.serve_forever()


if __name__ == "__main__":
    do_work()

