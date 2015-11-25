from contextlib import contextmanager
from mock import (
    ANY,
    patch
)
from unittest import TestCase

from jujupy import (
    EnvJujuClient,
    SimpleEnvironment,
    )
from quickstart_deploy import QuickstartTest


class TestQuickstartTest(TestCase):

    @contextmanager
    def from_args_cxt(self):
        with patch('jujupy.EnvJujuClient.get_version',
                   side_effect=lambda x, juju_path=None: ''):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                yield

    def test_from_args(self):
        def side_effect(x, y=None, debug=False):
            return x, y
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2
                )
        self.assertIs(type(quickstart), QuickstartTest)
        self.assertEqual(quickstart.client[0].environment, 'temp_env_name')
        self.assertEqual(quickstart.client[0].config, {
            'name': 'temp_env_name',
            })
        self.assertIs(quickstart.client[1], '/foo/bin/juju')
        self.assertEqual(quickstart.bundle_path, '/tmp/bundle.yaml')
        self.assertEqual(quickstart.log_dir, '/tmp/tmp')
        self.assertEqual(quickstart.service_count, 2)

    def test_from_args_agent_url(self):
        def side_effect(x, y=None, debug=False):
            return (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2, agent_url='http://agent_url.com'
                )
        self.assertEqual(quickstart.client[0].config['tools-metadata-url'],
                         'http://agent_url.com')

    def test_from_args_series(self):
        def side_effect(x, y=None, debug=False):
            return (x, y)
        with patch('jujupy.EnvJujuClient.by_version', side_effect=side_effect):
            with patch('jujupy.SimpleEnvironment.from_config',
                       side_effect=lambda x: SimpleEnvironment(x, {})):
                quickstart = QuickstartTest.from_args(
                    'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                    '/tmp/bundle.yaml', 2, series='precise'
                )
        self.assertEqual(quickstart.client[0].config['default-series'],
                         'precise')

    def test_from_args_debug(self):
        with self.from_args_cxt():
            quickstart = QuickstartTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '/tmp/bundle.yaml', 2, debug_flag=True
            )
        self.assertEqual(quickstart.client.debug, True)

    def test_from_args_region(self):
        with self.from_args_cxt():
            quickstart = QuickstartTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '/tmp/bundle.yaml', 2, region='region-foo')
        self.assertEqual(quickstart.client.env.config['region'], 'region-foo')

    def test_from_args_agent_stream(self):
        with self.from_args_cxt():
            quickstart = QuickstartTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '/tmp/bundle.yaml', 2, agent_stream='agent-stream-foo')
        self.assertEqual(quickstart.client.env.config['agent-stream'],
                         'agent-stream-foo')

    def test_from_args_bootstrap_host(self):
        with self.from_args_cxt():
            quickstart = QuickstartTest.from_args(
                'base_env', 'temp_env_name', '/foo/bin/juju', '/tmp/tmp',
                '/tmp/bundle.yaml', 2, bootstrap_host='host-foo')
        self.assertEqual(quickstart.client.env.config['bootstrap-host'],
                         'host-foo')

    def test_run_finally(self):
        def fake_iter_steps():
            yield {'bootstrap_host': 'foo'}
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        quickstart = QuickstartTest(client, '/tmp/bundle.yaml', '/tmp/logs', 2)
        with patch.object(client, 'destroy_environment') as qs_mock:
            with patch('quickstart_deploy.safe_print_status') as ps_mock:
                with patch('quickstart_deploy.dump_env_logs') as dl_mock:
                    with patch.object(quickstart, 'iter_steps',
                                      side_effect=fake_iter_steps):
                        quickstart.run()
        qs_mock.assert_called_once_with(delete_jenv=True)
        ps_mock.assert_called_once_with(client)
        dl_mock.assert_called_once_with(client, 'foo', '/tmp/logs')

    @patch('sys.stderr')
    def test_run_exception(self, se_mock):
        def fake_iter_steps():
            yield {'bootstrap_host': 'foo'}
            raise Exception()
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        quickstart = QuickstartTest(client, '/tmp/bundle.yaml', '/tmp/logs', 2)
        with patch.object(client, 'destroy_environment') as qs_mock:
            with patch('quickstart_deploy.safe_print_status') as ps_mock:
                with patch('quickstart_deploy.dump_env_logs') as dl_mock:
                    with patch.object(quickstart, 'iter_steps',
                                      side_effect=fake_iter_steps):
                        with self.assertRaises(BaseException):
                            quickstart.run()
        dl_mock.assert_called_once_with(client, 'foo', '/tmp/logs')
        qs_mock.assert_called_once_with(delete_jenv=True)
        ps_mock.assert_called_once_with(client)

    def test_iter_steps(self):
        client = EnvJujuClient(
            SimpleEnvironment('foo', {'type': 'local'}), '1.234-76', None)
        quickstart = QuickstartTest(client, '/tmp/bundle.yaml', '/tmp/logs', 2)
        steps = quickstart.iter_steps()
        with patch.object(client, 'quickstart') as qs_mock:
            # Test first yield
            step = steps.next()
        qs_mock.assert_called_once_with('/tmp/bundle.yaml')
        expected = {'juju-quickstart': 'Returned from quickstart'}
        self.assertEqual(expected, step)
        with patch('quickstart_deploy.get_machine_dns_name',
                   return_value='mocked_name') as dns_mock:
            # Test second yield
            step = steps.next()
        dns_mock.assert_called_once_with(client, '0')
        self.assertEqual('mocked_name', step['bootstrap_host'])
        with patch.object(client, 'wait_for_deploy_started') as wds_mock:
            # Test third yield
            step = steps.next()
        wds_mock.assert_called_once_with(2)
        self.assertEqual('Deploy stated', step['deploy_started'])
        with patch.object(client, 'wait_for_started') as ws_mock:
            # Test forth yield
            step = steps.next()
        ws_mock.assert_called_once_with(ANY)
        self.assertEqual('All Agents started', step['agents_started'])
