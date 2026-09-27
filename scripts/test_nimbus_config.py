"""Deployment boundaries: preview seeding, reserved branches, and safe bootstrap."""
import importlib.util
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.error

spec = importlib.util.spec_from_file_location('nimbus_config', Path(__file__).with_name('nimbus-config.py'))
config = importlib.util.module_from_spec(spec)
spec.loader.exec_module(config)


class PreviewConfigTests(unittest.TestCase):
    def test_preview_never_inherits_production_data_or_credentials(self):
        manifest = config.preview_manifest('https://0123456789abcdef.prayujt.com')
        self.assertIn('name: preview-ledger', manifest)
        self.assertIn('value: /data/ledger/main.beancount', manifest)
        self.assertIn('value: /app/preview', manifest)
        self.assertIn('value: beanframe-preview', manifest)
        self.assertNotIn('OIDC_CLIENT_SECRET', manifest)
        self.assertNotIn('beanframe.prayujt.com', manifest)
        self.assertNotIn('AUTH_DISABLED', manifest)

    def test_production_keeps_original_configuration(self):
        with tempfile.TemporaryDirectory() as tmp:
            target, output = Path(tmp)/'config.yaml', Path(tmp)/'output'
            with patch.dict(os.environ, {'BRANCH_NAME':'main','DEPLOY_CONFIG':str(target),'GITHUB_OUTPUT':str(output),'CLEANUP_BRANCH':''}), patch.object(config,'request') as request:
                config.main()
                request.assert_not_called()
            self.assertEqual(target.read_text(), Path('nimbus.yaml').read_text())
            self.assertIn('value: /app/example', target.read_text())

    def test_cannot_clean_production_or_its_namespace_alias(self):
        for branch in ('main','master',''):
            with self.subTest(branch=branch), patch.dict(os.environ, {'BRANCH_NAME':branch,'CLEANUP_BRANCH':'true'}), patch.object(config,'request') as request:
                with self.assertRaises(SystemExit): config.main()
                request.assert_not_called()

    def test_new_branch_bootstraps_then_uses_allocated_host(self):
        with tempfile.TemporaryDirectory() as tmp:
            target,output=Path(tmp)/'config.yaml',Path(tmp)/'output'
            responses=[urllib.error.HTTPError('url',404,'not found',{},None),{'ingress':'0123456789abcdef.prayujt.com'}]
            with patch.dict(os.environ,{'BRANCH_NAME':'feat/test','DEPLOY_CONFIG':str(target),'GITHUB_OUTPUT':str(output),'CLEANUP_BRANCH':''}),patch.object(config,'request',side_effect=responses),patch.object(config,'bootstrap') as bootstrap:
                config.main()
                bootstrap.assert_called_once_with('feat/test')
            self.assertIn('https://0123456789abcdef.prayujt.com',target.read_text())
            self.assertNotIn('preview.invalid',target.read_text())

    def test_cleanup_is_idempotent(self):
        error=urllib.error.HTTPError('url',404,'not found',{},None)
        with patch.dict(os.environ,{'BRANCH_NAME':'feat/test','CLEANUP_BRANCH':'true'}),patch.object(config,'request',side_effect=error) as request:
            config.main()
            self.assertEqual(request.call_args.kwargs,{'method':'DELETE'})
            self.assertIn('branch=feat%2Ftest',request.call_args.args[0])
