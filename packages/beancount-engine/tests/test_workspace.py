import concurrent.futures
import json
from pathlib import Path
import shutil
import tempfile
import unittest
from beancount_engine.workspace import snapshot, mutate, read_file, history, report, WorkspaceError

FIXTURE=Path(__file__).resolve().parents[3]/'testdata/ledger'
GOOD='2026-04-01 * "Workspace test"\n  Expenses:Hosting  15.25 USD\n  Assets:Checking  -15.25 USD\n'

class WorkspaceTests(unittest.TestCase):
    def setUp(self):
        self.temp=tempfile.TemporaryDirectory();self.addCleanup(self.temp.cleanup)
        self.root=Path(self.temp.name);shutil.copytree(FIXTURE,self.root,dirs_exist_ok=True)
        self.path=self.root/'main.beancount'
    def write(self,operation,**args):
        return mutate(self.path,{'operation':operation,'expected_revision':snapshot(self.path)['revision'],'actor':'test',**args})
    def test_create_update_delete_and_restore(self):
        before=snapshot(self.path)
        self.write('save_transaction',source=GOOD,file='transactions/2026.beancount')
        now=snapshot(self.path);self.assertEqual(len(now['transactions']),5);self.assertNotEqual(now['revision'],before['revision'])
        tx=next(t for t in now['transactions'] if t['narration']=='Workspace test')
        self.write('save_transaction',id=tx['id'],source=GOOD.replace('15.25','16.25'))
        now=snapshot(self.path);tx=next(t for t in now['transactions'] if t['narration']=='Workspace test')
        self.assertIn('16.25',tx['source']);self.assertEqual(len(now['transactions']),5)
        self.write('delete_transaction',id=tx['id']);self.assertEqual(len(snapshot(self.path)['transactions']),4)
        h=history(self.path)['entries'];self.assertEqual(len(h),3);self.assertNotIn('before_content',h[0]);self.assertEqual(h[0]['actor'],'test')
        self.write('restore_version',id=h[0]['id']);self.assertEqual(len(snapshot(self.path)['transactions']),5)
    def test_unbalanced_write_preserves_files_and_revision(self):
        before=snapshot(self.path)
        with self.assertRaises(WorkspaceError):self.write('save_transaction',source=GOOD.replace('-15.25','-1'))
        self.assertEqual(before['revision'],snapshot(self.path)['revision']);self.assertFalse(history(self.path)['entries'])
    def test_stale_and_missing_revision(self):
        old=snapshot(self.path)['revision'];self.write('save_transaction',source=GOOD)
        for rev in [old,'']:
            with self.assertRaises(WorkspaceError) as raised:mutate(self.path,{'operation':'save_transaction','expected_revision':rev,'source':GOOD})
            self.assertEqual(raised.exception.code,'aborted')
    def test_conflicting_processes_only_one_commits(self):
        rev=snapshot(self.path)['revision']
        # Independent processes model MCP/UI requests handled by separate writers.
        import subprocess,sys
        def attempt(_):
            p=subprocess.run([sys.executable,'-m','beancount_engine'],input=json.dumps({'path':str(self.path),'operation':'save_transaction','args':{'expected_revision':rev,'source':GOOD}}),text=True,capture_output=True,check=True)
            return json.loads(p.stdout)
        with concurrent.futures.ThreadPoolExecutor(2) as pool:results=list(pool.map(attempt,range(2)))
        self.assertEqual(sum('revision' in r for r in results),1);self.assertEqual(sum(r.get('code')=='aborted' for r in results),1)
    def test_path_and_configuration_boundaries(self):
        for name in ['../outside.beancount','/etc/passwd','.beancount-history/fake.json']:
            with self.assertRaises(WorkspaceError):read_file(self.path,name)
            with self.assertRaises(WorkspaceError):self.write('write_file',path=name,content='')
        original=read_file(self.path,'main.beancount')['content']
        for suffix in ['\nplugin "os"\n','\ninclude "../outside.beancount"\n','\noption "insert_pythonpath" "true"\n']:
            with self.assertRaises(WorkspaceError):self.write('write_file',path='main.beancount',content=original+suffix)
        with self.assertRaises(WorkspaceError):self.write('save_transaction',source=GOOD+'\nplugin "os"\n')
        (self.root/'linked.beancount').symlink_to('/etc/passwd')
        with self.assertRaises(WorkspaceError):snapshot(self.path)
    def test_file_edit_and_validation_repair(self):
        original=read_file(self.path,'main.beancount')
        self.write('write_file',path='main.beancount',content=original['content'].replace('Example ledger','Renamed ledger'))
        self.assertEqual(snapshot(self.path)['title'],'Renamed ledger')
        (self.root/'main.beancount').write_text(original['content']+'\n2026-05-01 * "bad"\n  Assets:Missing  1 USD\n')
        self.assertTrue(snapshot(self.path)['errors'])
        self.write('write_file',path='main.beancount',content=original['content'])
        self.assertFalse(snapshot(self.path)['errors'])
    def test_open_close_and_currency_restriction(self):
        self.write('open_account',name='Expenses:Testing',date='2026-01-01',currencies=['USD'])
        self.assertIn('Expenses:Testing',[a['name'] for a in snapshot(self.path)['accounts']])
        self.write('close_account',name='Expenses:Testing',date='2026-06-01')
        with self.assertRaises(WorkspaceError):self.write('close_account',name='Assets:Checking',date='2026-06-01')
        with self.assertRaises(WorkspaceError):self.write('open_account',name='Assets:Bad\nplugin "os"',date='2026-01-01',currencies=['USD'])
    def test_report_decimal_currency_and_date_semantics(self):
        usd=report(self.path,{'currency':'USD'});eur=report(self.path,{'currency':'EUR'})
        self.assertEqual(usd['expenses'],'12.34');self.assertEqual(eur['expenses'],'2.50')
        self.assertEqual(usd['assets'],'117.66');self.assertEqual(usd['liabilities'],'100.00')
        period=report(self.path,{'currency':'USD','from':'2026-02-01','to':'2026-02-28'})
        self.assertEqual(period['expenses'],'0');self.assertEqual(period['assets'],'117.66')
        with self.assertRaises(WorkspaceError):report(self.path,{'from':'2026-12-01','to':'2026-01-01'})
    def test_cost_basis_report(self):
        self.path.write_text('2026-01-01 open Assets:Stock\n2026-01-01 open Assets:Cash\n2026-01-02 * "Buy"\n  Assets:Stock  2 TEST {10 USD}\n  Assets:Cash -20 USD\n')
        result=report(self.path,{'currency':'USD'})
        self.assertEqual(result['assets'],'0');self.assertEqual(next(r for r in result['balance_sheet'] if r['account']=='Assets:Stock')['number'],'20')

if __name__=='__main__':unittest.main()
