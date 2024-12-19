#!/usr/bin/env python3

import csv
import os

##################################################
# CONSTANT                                       #
##################################################

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DATA_PATH = os.path.abspath(os.path.join(SCRIPT_DIR, '..', 'data', 'pigsty.csv'))
CATE_PATH = os.path.abspath(os.path.join(SCRIPT_DIR, '..', 'data', 'category.csv'))
DOCS_PATH = os.path.abspath(os.path.join(SCRIPT_DIR, '..', 'docs'))
STUB_PATH = os.path.abspath(os.path.join(SCRIPT_DIR, '..', 'stub'))

PG_VERS = ['17', '16', '15', '14', '13', '12']
DISTROS = ['el8', 'el9', 'd12', 'u22', 'u24']
DEB_OS = ['d12', 'u22', 'u24']
RPM_OS = ['el8', 'el9']

LICENSE_MAP = {
    'PostgreSQL': '**<span class="tcblue">PostgreSQL</span>**',
    'BSD 0-Clause': '**<span class="tcblue">BSD-0</span>**',
    'BSD 2-Clause': '**<span class="tcblue">BSD-2</span>**',
    'BSD 3-Clause': '**<span class="tcblue">BSD-3</span>**',
    'MIT': '**<span class="tcblue">MIT</span>**',
    'ISC': '**<span class="tcblue">ISC</span>**',
    'unrestricted': '**<span class="tcblue">Public</span>**',
    'Artistic': '**<span class="tccyan">Artistic</span>**',
    'Apache-2.0': '**<span class="tccyan">Apache-2</span>**',
    'MPL-2.0': '**<span class="tccyan">MPLv2</span>**',
    'GPL-2.0': '**<span class="tcwarn">GPLv2</span>**',
    'GPL-3.0': '**<span class="tcwarn">GPLv3</span>**',
    'LGPL-2.1': '**<span class="tcwarn">LGPLv2</span>**',
    'LGPL-3.0': '**<span class="tcwarn">LGPLv3</span>**',
    'AGPL-3.0': '**<span class="tcwarn">AGPLv3</span>**',
    'Timescale': '**<span class="tcwarn">Timescale</span>**',
}

REPO_MAP = {
    "PGDG": '**<span class="tccyan">PGDG</span>**',
    "PIGSTY": '**<span class="tcwarn">PIGSTY</span>**',
    "CONTRIB": '**<span class="tcblue">CONTRIB</span>**',
    "WILTON": '**<span class="tcpurple">WILTON</span>**',
    "CITUS": '**<span class="tcgreen">CITUS</span>**',
    "TIMESCALE": '**<span class="tcwarn">TIMESCALE</span>**'
}

REPO_CHECK_COLOR = {
    "PGDG": '**<span class="tccyan">✔</span>**',
    "PIGSTY": '**<span class="tcwarn">✔</span>**',
    "CONTRIB": '**<span class="tcblue">✔</span>**',
    "WILTON": '**<span class="tcpurple">✔</span>**',
    "CITUS": '**<span class="tcgreen">✔</span>**',
    "TIMESCALE": '**<span class="tcwarn">✔</span>**'
}

BLUE_CHECK = '<span class="tcblue">✔</span>'
WARN_CHECK = '<span class="tcwarn">✔</span>'
WARN_CROSS = '<span class="tcwarn">✘</span>'
RED_CROSS = '<span class="tcred">✘</span>'
RED_EXCLAM = '<span class="tcred">❗</span>'


THROW_LIST = []
HIDE_LIST = ['pgpool', 'plr', 'pgagent', 'dbt2', 'pgtap', 'faker', 'repmgr', 'slony', 'oracle_fdw', 'pg_strom', 'db2_fdw']
EXT_NOP_LIST = ["pg_mooncake", "citus"] # these extensions will be commented in pg_extensions due to conflict
DISTRO_MISS = {
    "el7": ["pg_dbms_job", "pljava"],
    "el8": ["pg_dbms_job", "pljava"],
    "el9": ["pg_dbms_job" ],
    "u24": ["pgml", "citus", "topn", "timescaledb_toolkit"],
    "u22": [],
    "u20": ["pljava"],
    "d12": [],
    "d11": ["pljava"],
}
DISTRO_FULLNAME = {
    "el7": "RHEL 7 Compatible",
    "el8": "RHEL 8 Compatible",
    "el9": "RHEL 9 Compatible",
    "u24": "Ubuntu 24.04 noble Compatible",
    "u22": "Ubuntu 24.04 jammy Compatible",
    "u20": "Ubuntu 24.04 focal Compatible",
    "d12": "Debian 12 bookworm Compatible",
    "d11": "Ubuntu 11 bullseye Compatible",
}

CATES = {
    "TIME": "TIME: TimescaleDB, Versioning & Temporal Table, Crontab, Async & Background Job Scheduler, ...",
    "GIS": "GIS: GeoSpatial Data Types, Operators, and Indexes, Hexagonal Indexing, OGR Data FDW, GeoIP & MobilityDB, etc...",
    "RAG": "RAG: Vector Database with IVFFLAT, HNSW, DiskANN Indexes, AI & ML in SQL interface, Similarity Funcs, etc... ",
    "FTS": "FTS: ElasticSearch Alternative with BM25, 2-gram/3-gram Fuzzy Search, Zhparser & Hunspell Segregation Dicts, etc...",
    "OLAP": "OLAP: DuckDB Integration with FDW & PG Lakehouse, Access Parquet from File/S3, Sharding with Citus/Partman/PlProxy, ...",
    "FEAT": "FEAT: OpenCypher with AGE, GraphQL, JsonSchema, Hints & Hypo Index, HLL, Rum, IVM, ChemRDKit, and Message Queues,...",
    "LANG": "LANG: Develop, Test, Package, and Deliver Stored Procedures written in various PL/Lanaguages: Java, Js, Lua, R, Sh, PRQL, ...",
    "TYPE": "TYPE: Dedicate New Data Types Like: prefix, sember, uint, SIUnit, RoaringBitmap, Rational, Sphere, Hash, RRule, and more...",
    "FUNC": "FUNC: Functionality such as sync/async HTTP, GZIP, JWT, SaltedHash, Extra Window Aggs, PCRE, ICU, ID & Rand Generator, etc...",
    "ADMIN": "ADMIN: Utilities for Bloat Control, DirtyRead, BufferInspect, DDL Generate, ChecksumVerify, Permission, Priority, Catalog,...",
    "STAT": "STAT: Observability Catalogs, Monitoring Metrics & Views, Statistics, Query Plans, WaitSampling, SlowLogs, and etc...",
    "SEC": "SEC: Auditing Logs, Enforce Passwords, Keep Secrets, TDE, SM Algorithm, Login Hooks, Log Erros, Extension White List, ...",
    "FDW": "FDW: Wrappers & Multicorn for FDW Development, Access other DBMS: MySQL, Mongo, SQLite, MSSQL, Oracle, HDFS, DB2,...",
    "SIM": "SIM: Protocol Simulation & heterogeneous DBMS Compatibility: Oracle, MSSQL, DB2, MySQL, Memcached, and Babelfish!",
    "ETL": "ETL: Logical Replication, Decoding, CDC in protobuf/JSON/Mongo format, Copy & Load & Compare Postgres Databases,...",
}

CATE_LIST = ["TIME","GIS","RAG","FTS","OLAP","FEAT","LANG","TYPE","FUNC","ADMIN","STAT","SEC","FDW","SIM","ETL"]

COLS = {
    "id":         {'header': 'ID'         ,'center': True,  'func': lambda row: str(row['id'])         },
    "name":       {'header': 'name'       ,'center': True,  'func': lambda row: str(row['name'])       },
    "alias":      {'header': 'alias'      ,'center': True,  'func': lambda row: str(row['alias'])      },
    "category":   {'header': 'category'   ,'center': True,  'func': lambda row: str(row['category'])   },
    "url":        {'header': 'url'        ,'center': True,  'func': lambda row: str(row['url'])        },
    "license":    {'header': 'license'    ,'center': True,  'func': lambda row: str(row['license'])    },
    "tags":       {'header': 'tags'       ,'center': True,  'func': lambda row: str(row['tags'])       },
    "version":    {'header': 'version'    ,'center': True,  'func': lambda row: str(row['version'])    },
    "repo":       {'header': 'repo'       ,'center': True,  'func': lambda row: str(row['repo'])       },
    "lang":       {'header': 'lang'       ,'center': True,  'func': lambda row: str(row['lang'])       },
    "utility":    {'header': 'utility'    ,'center': True,  'func': lambda row: str(row['utility'])    },
    "lead":       {'header': 'lead'       ,'center': True,  'func': lambda row: str(row['lead'])       },
    "has_solib":  {'header': 'has_solib'  ,'center': True,  'func': lambda row: str(row['has_solib'])  },
    "need_ddl":   {'header': 'need_ddl'   ,'center': True,  'func': lambda row: str(row['need_ddl'])   },
    "need_load":  {'header': 'need_load'  ,'center': True,  'func': lambda row: str(row['need_load'])  },
    "trusted":    {'header': 'trusted'    ,'center': True,  'func': lambda row: str(row['trusted'])    },
    "relocatable":{'header': 'relocatable','center': True,  'func': lambda row: str(row['relocatabl']) },
    "schemas":    {'header': 'schemas'    ,'center': True,  'func': lambda row: str(row['schemas'])    },
    "pg_ver":     {'header': 'pg_ver'     ,'center': True,  'func': lambda row: str(row['pg_ver'])     },
    "requires":   {'header': 'requires'   ,'center': True,  'func': lambda row: str(row['requires'])   },
    "rpm_ver":    {'header': 'rpm_ver'    ,'center': True,  'func': lambda row: str(row['rpm_ver'])    },
    "rpm_repo":   {'header': 'rpm_repo'   ,'center': True,  'func': lambda row: str(row['rpm_repo'])   },
    "rpm_pkg":    {'header': 'rpm_pkg'    ,'center': True,  'func': lambda row: str(row['rpm_pkg'])    },
    "rpm_deps":   {'header': 'rpm_deps'   ,'center': True,  'func': lambda row: str(row['rpm_deps'])   },
    "deb_ver":    {'header': 'deb_ver'    ,'center': True,  'func': lambda row: str(row['deb_ver'])    },
    "deb_repo":   {'header': 'deb_repo'   ,'center': True,  'func': lambda row: str(row['deb_repo'])   },
    "deb_pkg":    {'header': 'deb_pkg'    ,'center': True,  'func': lambda row: str(row['deb_pkg'])    },
    "deb_deps":   {'header': 'deb_deps'   ,'center': True,  'func': lambda row: str(row['deb_deps'])   },
    "en_desc":    {'header': 'Description','center': False, 'func': lambda row: str(row['en_desc'])    },
    "zh_desc":    {'header': '说明'        ,'center': False, 'func': lambda row: str(row['zh_desc'])    },
    "comment":    {'header': 'Comment'    ,'center': False, 'func': lambda row: str(row['comment'])    },

    "ext":        {'header': 'Extension'  ,'center': False, 'func': lambda row: "[%s](%s)" % (row["name"], row["url"])  },
    "ext2":       {'header': 'Extension'  ,'center': False, 'func': lambda row: row["name"] },
    "ext3":       {'header': 'Extension'  ,'center': False, 'func': lambda row: "[%s](/%s)" % (row["name"], row['name'])  },
    "ext4":       {'header': 'Extension'  ,'center': False, 'func': lambda row: "[`%s`](/%s)" % (row["name"], row['name'])  },
    "link":       {'header': 'Website'    ,'center': True,  'func': lambda row: "[LINK](%s)" % row["url"]  },
    "pkg":        {'header': 'Package'    ,'center': False, 'func': lambda row: "[%s](/%s)" % (row['alias'], row['name'])  },
    "pkg2":       {'header': 'Package'    ,'center': False, 'func': lambda row: "[%s](%s)" % (row['alias'], row['url'])  },
    "pkg3":       {'header': 'Alias'      ,'center': False, 'func': lambda row: "[%s](/%s)" % (row['alias'], row['name'])  },
    "ver":        {'header': 'Version'    ,"center": True,  "func": lambda row: row['version'] },
    "rpmver":     {'header': 'Version'    ,"center": False, "func": lambda row: row['rpm_ver'] },
    "debver":     {'header': 'Version'    ,"center": False, "func": lambda row: row['deb_ver'] },
    "cat":        {'header': 'Category'   ,"center": True,  "func": lambda row: "[%s](/%s)" % (row['category'], row['category'].lower()) },
    "lic":        {'header': 'License'    ,"center": True,  "func": lambda row: LICENSE_MAP.get(row['license'], row['license']) },
    "lan":        {'header': 'PL'         ,'center': True,  'func': lambda row: '' if not row['lang'] else '`%s`' % row['lang'] },
    "rpmrepo":    {'header': 'RPM'        ,"center": True,  "func": lambda row: REPO_MAP.get(row['rpm_repo'], row['rpm_repo']) },
    "rpmrepo2":   {'header': 'REPO'       ,"center": True,  "func": lambda row: REPO_MAP.get(row['rpm_repo'], row['rpm_repo']) },
    "debrepo":    {'header': 'DEB'        ,"center": True,  "func": lambda row: REPO_MAP.get(row['deb_repo'], row['deb_repo']) },
    "debrepo2":   {'header': 'REPO'       ,"center": True,  "func": lambda row: REPO_MAP.get(row['deb_repo'], row['deb_repo']) },
    "rpmpkg":     {'header': 'RPM Package',"center": False, "func": lambda row: '`%s`' % row['rpm_pkg'] },
    "debpkg":     {'header': 'DEB Package',"center": False, "func": lambda row: '`%s`' % row['deb_pkg'] },
    "rpmpkg2":    {'header': 'Package Pattern',"center": False, "func": lambda row: '`%s`' % row['rpm_pkg'] },
    "debpkg2":    {'header': 'Package Pattern',"center": False, "func": lambda row: '`%s`' % row['deb_pkg'] },
    "rpmpkg3":    {'header': 'RPM Package',"center": False, "func": lambda row: '<br>'.join(['`%s`'%i for i in  row['rpm_pkg'].split(' ') ]) },
    "debpkg3":    {'header': 'DEB Package',"center": False, "func": lambda row: '<br>'.join(['`%s`'%i for i in  row['deb_pkg'].split(' ') ]) },
    "r17":        {'header': '17'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['rpm_repo'], BLUE_CHECK) if '17' in row['rpm_pg'] else '' },
    "r16":        {'header': '16'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['rpm_repo'], BLUE_CHECK) if '16' in row['rpm_pg'] else '' },
    "r15":        {'header': '15'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['rpm_repo'], BLUE_CHECK) if '15' in row['rpm_pg'] else '' },
    "r14":        {'header': '14'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['rpm_repo'], BLUE_CHECK) if '14' in row['rpm_pg'] else '' },
    "r13":        {'header': '13'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['rpm_repo'], BLUE_CHECK) if '13' in row['rpm_pg'] else '' },
    "r12":        {'header': '12'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['rpm_repo'], BLUE_CHECK) if '12' in row['rpm_pg'] else '' },
    "d17":        {'header': '17'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if '17' in row['deb_pg'] else '' },
    "d16":        {'header': '16'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if '16' in row['deb_pg'] else '' },
    "d15":        {'header': '15'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if '15' in row['deb_pg'] else '' },
    "d14":        {'header': '14'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if '14' in row['deb_pg'] else '' },
    "d13":        {'header': '13'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if '13' in row['deb_pg'] else '' },
    "d12":        {'header': '12'         ,"center": True,  "func": lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if '12' in row['deb_pg'] else '' },
    "bin":        {'header': '`Bin`'      ,'center': True,  'func': lambda row: REPO_CHECK_COLOR.get(row['deb_repo'], BLUE_CHECK) if row['utility'] else ''   },
    "load":       {'header': '`LOAD`'     ,"center": True,  "func": lambda row: '' if row['need_load'] is None else (RED_EXCLAM if row['need_load'] else '' ) },
    "ddl":        {'header': '`DDL`'      ,"center": True,  "func": lambda row: '' if row['need_ddl' ] is None else (BLUE_CHECK if row['need_ddl' ] else WARN_CROSS) },
    "trust":      {'header': '`TRUST`'    ,"center": True,  "func": lambda row: '' if row['trusted'  ] is None else (BLUE_CHECK if row['trusted'  ] else WARN_CROSS) },
    "reloc":      {'header': '`RELOC`'    ,"center": True,  "func": lambda row: '' if row['relocatable'] is None else (BLUE_CHECK if row['relocatable'] else WARN_CROSS) },
    "dylib":      {'header': '`DYLIB`'    ,"center": True,  "func": lambda row: '' if row['has_solib'  ] is None else (BLUE_CHECK if row['has_solib'  ] else WARN_CROSS) },
    "distro":     {'header': 'OS'         ,"center": True,  "func": lambda row: 'Distro-' + row['name'] },
    "req":        {'header': 'Requires'   ,'center': False, 'func': lambda row: ', '.join([ '[`%s`](%s)'%(e,e) for e in row['requires'] ])  },
    "reqd":       {'header': 'Required by','center': False, 'func': lambda row: ', '.join ([ '[`%s`](/%s)'%(i,i) for i in DEP_MAP[row['name']]]) if row['name'] in DEP_MAP else ''  },

    "tag":        {'header': 'Tags'       ,'center': False, 'func': lambda row: ', '.join([ '`%s`'%e for e in row['tags'] ])  },
    "schema":     {'header': 'Schemas'    ,'center': False, 'func': lambda row: ', '.join([ '`%s`'%e for e in row['schemas'] ])  },
    "rpmdep":     {'header': 'Dependency' ,'center': False, 'func': lambda row: ', '.join([ '`%s`'%e for e in row['rpm_deps'] ])  },
    "debdep":     {'header': 'Dependency' ,'center': False, 'func': lambda row: ', '.join([ '`%s`'%e for e in row['deb_deps'] ])  },
}

def getcol(col, ext):
    return COLS[col]['func'](ext)

# generate column descriptor list
def Columns(columns):
    return [COLS.get(i) for i in columns]


# generate markdown table
def get_markdown_table(header, data):
    headers = "| " + " | ".join(header) + " |\n"
    separator = "|" + "|".join([ ':' + '-' * len(h)+':' for h in header]) + "|\n"
    rows = [headers, separator]
    for row in data:
        row_str = "| " + " | ".join(row) + " |\n"
        rows.append(row_str)
    return ''.join(rows)

def tabulate(cols, filter_func):
    headers = "| " + " | ".join([col['header'] for col in cols]) + " |\n"
    separator = "|" + "|".join([ ':' + '-' * len(col['header'])+':' if col['center'] else '-' * (len(col['header'])+2) for col in cols]) + "|\n"
    rows = [headers, separator]
    for row in DATA:
        if not filter_func(row): continue
        row_values = [col['func'](row) for col in cols]
        row_str = "| " + " | ".join(row_values) + " |\n"
        rows.append(row_str)
    return ''.join(rows)

# open file for write
def openw(p):
    output_path = os.path.join(DOCS_PATH, p)
    return open(output_path, 'w')

# load data from pigsty extension csv, or default to ../data/ext.csv
def load_data(filepath=DATA_PATH):
    parse_array = lambda v: v[1:-1].split(',') if v.startswith('{') and v.endswith('}') else []
    data = []
    with open(filepath, newline='', encoding='utf-8') as csvfile:
        reader = csv.DictReader(csvfile)
        for row in reader:
            row['id'] = int(row['id'])
            if row['tags']: row['tags'] = parse_array(row['tags'])
            if row['schemas']: row['schemas'] = parse_array(row['schemas'])
            if row['pg_ver']: row['pg_ver'] = parse_array(row['pg_ver'])
            if row['requires']: row['requires'] = parse_array(row['requires'])
            if row['rpm_deps']: row['rpm_deps'] = parse_array(row['rpm_deps'])
            if row['deb_deps']: row['deb_deps'] = parse_array(row['deb_deps'])
            if row['rpm_pg']: row['rpm_pg'] = parse_array(row['rpm_pg'])
            if row['deb_pg']: row['deb_pg'] = parse_array(row['deb_pg'])

            row['utility']     =  True if row['utility']   == 't'  else False if row['utility'] == 'f'  else None
            row['lead']        =  True if row['lead']   == 't'  else False if row['lead'] == 'f'  else None
            row['has_solib']   =  True if row['has_solib']   == 't'  else False if row['has_solib'] == 'f'  else None
            row['need_ddl']    =  True if row['need_ddl']    == 't'  else False if row['need_ddl'] == 'f'  else None
            row['need_load']   =  True if row['need_load']   == 't'  else False if row['need_load'] == 'f'  else None
            row['trusted']     =  True if row['trusted']     == 't'  else False if row['trusted'] == 'f'  else None
            row['relocatable'] =  True if row['relocatable'] == 't'  else False if row['relocatable'] == 'f'  else None

            row['has_rpm'] = True if row['rpm_repo'] else False
            row['has_deb'] = True if row['deb_repo'] else False
            row['has_both'] = True if row['deb_repo'] and row['deb_repo'] else False
            row['pg12'] = True if '12' in row['pg_ver'] else False
            row['pg13'] = True if '13' in row['pg_ver'] else False
            row['pg14'] = True if '14' in row['pg_ver'] else False
            row['pg15'] = True if '15' in row['pg_ver'] else False
            row['pg16'] = True if '16' in row['pg_ver'] else False
            row['pg17'] = True if '17' in row['pg_ver'] else False

            row['contrib'] = True if row['repo'] == 'CONTRIB' else False
            row['rpm_pgdg'] = True if row['has_rpm'] and row['rpm_repo'] == 'PGDG' else False
            row['rpm_pigsty'] = True if row['has_rpm'] and row['rpm_repo'] == 'PIGSTY' else False
            row['rpm_misc'] = True if row['has_rpm'] and (row['rpm_repo'] not in ('PGDG', 'PIGSTY', 'TIMESCALE', 'CITUS','CONTRIB','')) else False

            row['deb_pgdg'] = True if row['has_deb'] and row['deb_repo'] == 'PGDG' else False
            row['deb_pigsty'] = True if row['has_deb'] and row['deb_repo'] == 'PIGSTY' else False
            row['deb_misc'] = True if row['has_deb'] and (row['deb_repo'] not in ('PGDG', 'PIGSTY', 'TIMESCALE', 'CITUS','CONTRIB','')) else False
            data.append(row)
    return data

# load data from pigsty extension csv, or default to ../data/ext.csv
def load_categories(filepath=CATE_PATH):
    data = []
    with open(filepath, newline='', encoding='utf-8') as csvfile:
        reader = csv.DictReader(csvfile)
        for row in reader:
            data.append(row)
    return data



def stat_data(data):
    res = {"stat": {}, "rpm_ext": {}, "deb_ext": {}, "rpm_pkg": {}, "deb_pkg": {}}
    res["stat"]["all"] = len([i for i in data ])
    res["stat"]["rpm"] = len([i for i in data if i['has_rpm'] ])
    res["stat"]["deb"] = len([i for i in data if i['has_deb'] ])
    res["stat"]["both"] = len([i for i in data if i['has_rpm'] and i['has_deb'] ])
    res["stat"]["contrib"] = len([i for i in data if i['repo'] == 'CONTRIB'])
    res["stat"]["non-contrib"] =  len([i for i in data ]) - len([i for i in data if i['contrib']])

    res["rpm_ext"]["all"] = len([i for i in data if i['has_rpm']])
    res["rpm_pkg"]["all"] = len(set([i['rpm_pkg'] for i in data if i['has_rpm']]))
    res["deb_ext"]["all"] = len([i for i in data if i['has_deb']])
    res["deb_pkg"]["all"] = len(set([i['deb_pkg'] for i in data if i['has_deb']]))

    res["rpm_ext"]["pgdg"]   = len([i['name'] for i in data if i['has_rpm'] and i['rpm_pgdg'] ])
    res["rpm_ext"]["pigsty"] = len([i['name'] for i in data if i['has_rpm'] and i['rpm_pigsty']])
    res["rpm_ext"]["contrib"]= len([i['name'] for i in data if i['contrib'] ])
    res["rpm_ext"]["misc"]   = len([i['name'] for i in data if i['has_rpm'] and i['rpm_misc']])
    res["rpm_ext"]["miss"]   = len([i['name'] for i in data if not i['has_rpm']])
    res["rpm_ext"]["pg12"]   = len([i['name'] for i in data if i['has_rpm'] and '12' in i['rpm_pg'] ])
    res["rpm_ext"]["pg13"]   = len([i['name'] for i in data if i['has_rpm'] and '13' in i['rpm_pg'] ])
    res["rpm_ext"]["pg14"]   = len([i['name'] for i in data if i['has_rpm'] and '14' in i['rpm_pg'] ])
    res["rpm_ext"]["pg15"]   = len([i['name'] for i in data if i['has_rpm'] and '15' in i['rpm_pg'] ])
    res["rpm_ext"]["pg16"]   = len([i['name'] for i in data if i['has_rpm'] and '16' in i['rpm_pg'] ])
    res["rpm_ext"]["pg17"]   = len([i['name'] for i in data if i['has_rpm'] and '17' in i['rpm_pg'] ])

    res["rpm_pkg"]["pgdg"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and i['rpm_pgdg'] ]))
    res["rpm_pkg"]["pigsty"] = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and i['rpm_pigsty']]))
    res["rpm_pkg"]["contrib"]= 1
    res["rpm_pkg"]["misc"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and i['rpm_misc']]))
    res["rpm_pkg"]["miss"]   = len(set([i['rpm_pkg'] for i in data if not i['has_rpm']]))
    res["rpm_pkg"]["pg12"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and '12' in i['rpm_pg']]))
    res["rpm_pkg"]["pg13"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and '13' in i['rpm_pg']]))
    res["rpm_pkg"]["pg14"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and '14' in i['rpm_pg']]))
    res["rpm_pkg"]["pg15"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and '15' in i['rpm_pg']]))
    res["rpm_pkg"]["pg16"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and '16' in i['rpm_pg']]))
    res["rpm_pkg"]["pg17"]   = len(set([i['rpm_pkg'] for i in data if i['has_rpm'] and '17' in i['rpm_pg']]))

    res["deb_ext"]["pgdg"]   = len([i['name'] for i in data if i['has_deb'] and i['deb_pgdg'] ])
    res["deb_ext"]["pigsty"] = len([i['name'] for i in data if i['has_deb'] and i['deb_pigsty']])
    res["deb_ext"]["contrib"]= len([i['name'] for i in data if i['contrib'] ])
    res["deb_ext"]["misc"]   = len([i['name'] for i in data if i['has_deb'] and i['deb_misc']])
    res["deb_ext"]["miss"]   = len([i['name'] for i in data if not i['has_deb']])
    res["deb_ext"]["pg12"]   = len([i['name'] for i in data if i['has_deb'] and '12' in i['deb_pg']])
    res["deb_ext"]["pg13"]   = len([i['name'] for i in data if i['has_deb'] and '13' in i['deb_pg']])
    res["deb_ext"]["pg14"]   = len([i['name'] for i in data if i['has_deb'] and '14' in i['deb_pg']])
    res["deb_ext"]["pg15"]   = len([i['name'] for i in data if i['has_deb'] and '15' in i['deb_pg']])
    res["deb_ext"]["pg16"]   = len([i['name'] for i in data if i['has_deb'] and '16' in i['deb_pg']])
    res["deb_ext"]["pg17"]   = len([i['name'] for i in data if i['has_deb'] and '17' in i['deb_pg']])

    res["deb_pkg"]["pgdg"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and i['deb_pgdg'] ]))
    res["deb_pkg"]["pigsty"] = len(set([i['deb_pkg'] for i in data if i['has_deb'] and i['deb_pigsty']]))
    res["deb_pkg"]["contrib"]= 1
    res["deb_pkg"]["misc"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and i['deb_misc']]))
    res["deb_pkg"]["miss"]   = len(set([i['deb_pkg'] for i in data if not i['has_deb']]))
    res["deb_pkg"]["pg12"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and '12' in i['deb_pg'] ]))
    res["deb_pkg"]["pg13"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and '13' in i['deb_pg'] ]))
    res["deb_pkg"]["pg14"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and '14' in i['deb_pg'] ]))
    res["deb_pkg"]["pg15"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and '15' in i['deb_pg'] ]))
    res["deb_pkg"]["pg16"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and '16' in i['deb_pg'] ]))
    res["deb_pkg"]["pg17"]   = len(set([i['deb_pkg'] for i in data if i['has_deb'] and '17' in i['deb_pg'] ]))

    return res


DATA = load_data()
STAT = stat_data(DATA)
CATE = list(dict.fromkeys([i['category'] for i in DATA]))
DEP_MAP = {}
EXT_MAP = {}
for ext in DATA:
    EXT_MAP[ext['name']] = ext
    if ext['requires']:
        for e in ext['requires']:
            if e not in DEP_MAP:
                DEP_MAP[e] = [ext['name']]
            else:
                DEP_MAP[e].append(ext['name'])


def tabulate_stats(todolist):
    entries = {
        'rpm_ext': 'RPM Extension',
        'deb_ext': 'DEB Extension',
        'rpm_pkg': 'RPM Package',
        'deb_pkg': 'DEB Package'
    }
    filters = ['all', 'pgdg', 'pigsty', 'contrib', 'misc', 'miss', 'pg17', 'pg16', 'pg15', 'pg14', 'pg13', 'pg12']
    headers = ['Entry / Filter', 'All', 'PGDG', 'PIGSTY', 'CONTRIB', 'MISC', 'MISS', 'PG17', 'PG16', 'PG15', 'PG14', 'PG13', 'PG12']
    markdown = '|' + ' | '.join(headers) + '|\n'
    markdown += '|' + '|'.join([':----:' for _ in headers]) + '|\n'
    for key in todolist:
        row = [entries[key]]
        for filt in filters:
            value = STAT.get(key, {}).get(filt, '')
            row.append(str(value))
        markdown += '| ' + ' | '.join(row) + ' |\n'
    return markdown


def process_ext(ver, distro, ext):
    if distro in ('rpm', 'el7', 'el8', 'el9'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY,  = 'rpm_repo', 'rpm_pkg', 'rpm_pg', 'has_rpm'
    elif distro in ('deb', 'u20', 'u22', 'u24', 'd12', 'd11'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY, = 'deb_repo', 'deb_pkg', 'deb_pg',  'has_deb'
    else:
        raise ValueError("Invalid distro: %s" % distro)
    name, alias, extension, package, pg_vers, avail = ext['name'], ext['alias'], ext['alias'], ext[PKG_KEY].replace('$v', ver), ext[VER_KEY], ext[HAS_KEY]
    hide_pkg, hide_ext, drop_pkg, drop_ext = True, True, False, False
    if avail and ver in pg_vers and name not in DISTRO_MISS[distro]:
        hide_pkg, hide_ext = False, False

    # rename extension field in certain cases
    if name == 'pgaudit' and distro in RPM_OS and ver in ['12','13','14','15']:
        # pgaudit bad case: pg16+ = pgaudit, pg15=pgaudit17, pg14=pgaudit16 pg13=pgaudit15 pg12=pgaudit14
        package, extension = package.replace('pgaudit', 'pgaudit' + str(int(ver)+2)), alias + str(int(ver)+2)
    if name == 'citus' and distro in DEB_OS and ver in ['12', '13']:
        package, extension = package.replace('citus-12.1', 'citus-' + ('10.2' if ver == '12' else '11.3') ), alias + str(int(ver)-2)
    if name == 'postgis' and distro in ['el8', 'el9'] and ver == '12': # el8/9 will use postgis34 for pg12
        package, extension = package.replace('postgis35', 'postgis34'), 'postgis34'
    if name == 'postgis' and distro == 'el7': # el7 with postgis33
        package, extension = package.replace('postgis35', 'postgis33'), 'postgis33'
    if name in ['pg_mooncake', 'citus']:
        hide_ext = True

    # ubuntu 24.04 bad case
    if distro == 'u24' and name == 'timescaledb' and ver == '12': # no timescaledb 12 for ubuntu24
        hide_pkg, hide_ext = True, True
    if distro == 'u24' and name in ['pg_partman', 'timeseries'] and ver in ['12','13']: # not pg_partman 12,13 for u24
        hide_pkg, hide_ext = True, True

    # version ad hoc logic

    # just don't want them
    if alias in THROW_LIST:
        drop_pkg, drop_ext = True, True
    if alias in HIDE_LIST:
        hide_pkg, hide_ext = True, True

    # merge babelfish pkg & ext into one wiltondb package/extension
    if name == 'babelfishpg_common' and distro in ['el7', 'el8', 'el9', 'u20', 'u22', 'u24']:
        package, extension = 'wiltondb', 'wiltondb'
        hide_pkg, hide_ext = True, True
    if name in ['babelfishpg_tsql','babelfishpg_tds','babelfishpg_money']:
        drop_pkg, drop_ext = True, True
    if name.startswith('babelfishpg') and distro not in ['el7', 'el8', 'el9', 'u20', 'u22', 'u24']:
        drop_pkg, drop_ext = True, True   # wiltondb not available on other platforms

    # merge hunspell pkg & ext into one wiltondb package/extension
    if name == 'hunspell_cs_cz':
        extension = 'hunspell'
    if name.startswith('hunspell') and name != 'hunspell_cs_cz':
        drop_ext = True # package still need to be downloaded

    pkg_aye, pkg_nay, ext_aye, ext_nay = [], [], [], []
    if not drop_pkg:
        if hide_pkg: pkg_nay.append('#' + package)
        else: pkg_aye.append(package)
    if not drop_ext:
        if hide_ext: ext_nay.append('#' + extension)
        else: ext_aye.append(extension)

    return pkg_aye, pkg_nay, ext_aye, ext_nay


# return (pkg_name, ext_name, available)
def judge_ext(ver, distro, ext):
    if distro in ('rpm', 'el7', 'el8', 'el9'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY,  = 'rpm_repo', 'rpm_pkg', 'rpm_pg', 'has_rpm'
    elif distro in ('deb', 'u20', 'u22', 'u24', 'd12', 'd11'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY, = 'deb_repo', 'deb_pkg', 'deb_pg',  'has_deb'
    else:
        raise ValueError("Invalid distro: %s" % distro)
    name, alias, extension, package, pg_vers, avail = ext['name'], ext['alias'], ext['alias'], ext[PKG_KEY].replace('$v', ver), ext[VER_KEY], ext[HAS_KEY]
    avail = str(ver) in pg_vers

    if name in DISTRO_MISS[distro]:
        avail = False
    # rename extension field in certain cases
    if name == 'pgaudit' and distro in RPM_OS and ver in ['12','13','14','15']:
        # pgaudit bad case: pg16+ = pgaudit, pg15=pgaudit17, pg14=pgaudit16 pg13=pgaudit15 pg12=pgaudit14
        package, extension = package.replace('pgaudit', 'pgaudit' + str(int(ver)+2)), alias + str(int(ver)+2)
    if name == 'citus' and distro in DEB_OS and ver in ['12', '13']:
        package, extension = package.replace('citus-12.1', 'citus-' + ('10.2' if ver == '12' else '11.3') ), alias + str(int(ver)-2)
    if name == 'postgis' and distro in ['el8', 'el9'] and ver == '12': # el8/9 will use postgis34 for pg12
        package, extension = package.replace('postgis35', 'postgis34'), 'postgis34'
    if name == 'postgis' and distro == 'el7': # el7 with postgis33
        package, extension = package.replace('postgis35', 'postgis33'), 'postgis33'
    if name.startswith('babelfishpg'):
        package, extension = 'wiltondb', 'wiltondb'
    # ubuntu 24.04 bad case
    if distro == 'u24' and name == 'timescaledb' and ver == '12': avail = False
    if distro == 'u24' and name in ['pg_partman', 'timeseries'] and ver in ['12','13']: avail = False
    if alias in THROW_LIST: avail = False

    return package, extension, avail



def avail_matrix(extname, cell='avail'):
    ext = EXT_MAP[extname]
    header = ['Distro / Ver', 'PG17', 'PG16', 'PG15', 'PG14', 'PG13', 'PG12']
    data = []
    for distro in DISTROS:
        row = ['`%s`' % distro]
        for ver in PG_VERS:
            package, extension, avail = judge_ext(ver, distro, ext)
            if cell == 'avail':
                row.append( WARN_CHECK if extension != ext['alias'] else (BLUE_CHECK if avail else RED_CROSS) )
            elif cell == 'pkg':
                if avail:
                    row.append( '%s' % '<br>'.join(['`%s`' % i for i in  package.split(' ')]) )
                else:
                    row.append(RED_CROSS)
            elif cell == 'ext':
                if avail:
                    row.append( '`%s`' % extension )
                else:
                    row.append(RED_CROSS)

        data.append(row)
    return get_markdown_table(header, data)



# generate postgres related repo_package list and pg_extension according to pg major version and os distro
def gen_ext_list(ver, distro):
    if distro.lower() in ('rpm', 'el7', 'el8', 'el9'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY,  = 'rpm_repo', 'rpm_pkg', 'rpm_pg', 'has_rpm'
    elif distro.lower() in ('deb', 'u20', 'u22', 'u24', 'd12', 'd11'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY, = 'deb_repo', 'deb_pkg', 'deb_pg',  'has_deb'
    else:
        raise ValueError("Invalid distro: %s" % distro)

    # generate pkg & ext list, per category
    repo_pkg, ext_list = [], []
    for cate in CATE:
        pkg_aye, pkg_nay, ext_aye, ext_nay = [], [], [], []
        for ext in [e for e in DATA if e['category'] == cate and e[REPO_KEY] != 'CONTRIB' and e['lead']]:
            #ext['package'], ext['pg_ver'], ext['avail'] =  ext[PKG_KEY].replace('$v', ver), ext[VER_KEY], ext[HAS_KEY]
            t_pkg_aye, t_pkg_nay, t_ext_aye, t_ext_nay = process_ext(ver, distro, ext)
            pkg_aye.extend(t_pkg_aye)
            pkg_nay.extend(t_pkg_nay)
            ext_aye.extend(t_ext_aye)
            ext_nay.extend(t_ext_nay)

        repo_entry = (' '.join(list(dict.fromkeys(pkg_aye))) + ' ' + ' '.join(list(dict.fromkeys(pkg_nay)))).rstrip('# ')
        ext_entry = (' '.join(list(dict.fromkeys(ext_aye))) + ' ' + ' '.join(list(dict.fromkeys(ext_nay)))).rstrip('# ')
        repo_pkg.append(repo_entry)
        ext_list.append(ext_entry.replace('#babelfishpg_common #babelfishpg_tsql #babelfishpg_tds #babelfishpg_money', '#wiltondb'))

    return repo_pkg, ext_list


def gen_cate_ext_list(ver, distro, cate):
    if distro.lower() in ('rpm', 'el7', 'el8', 'el9'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY,  = 'rpm_repo', 'rpm_pkg', 'rpm_pg', 'has_rpm'
    elif distro.lower() in ('deb', 'u20', 'u22', 'u24', 'd12', 'd11'):
        REPO_KEY, PKG_KEY, VER_KEY, HAS_KEY, = 'deb_repo', 'deb_pkg', 'deb_pg',  'has_deb'
    else:
        raise ValueError("Invalid distro: %s" % distro)

    # generate pkg & ext list, per category
    repo_pkg, ext_list = [], []
    pkg_aye, pkg_nay, ext_aye, ext_nay = [], [], [], []
    for ext in [e for e in DATA if e['category'] == cate and e[REPO_KEY] != 'CONTRIB' and e['lead']]:
        #ext['package'], ext['pg_ver'], ext['avail'] =  ext[PKG_KEY].replace('$v', ver), ext[VER_KEY], ext[HAS_KEY]
        t_pkg_aye, t_pkg_nay, t_ext_aye, t_ext_nay = process_ext(ver, distro, ext)
        pkg_aye.extend(t_pkg_aye)
        pkg_nay.extend(t_pkg_nay)
        ext_aye.extend(t_ext_aye)
        ext_nay.extend(t_ext_nay)

    repo_entry = (' '.join(list(dict.fromkeys(pkg_aye))) + ' ' + ' '.join(list(dict.fromkeys(pkg_nay)))).rstrip('# ')
    ext_entry = (' '.join(list(dict.fromkeys(ext_aye))) + ' ' + ' '.join(list(dict.fromkeys(ext_nay)))).rstrip('# ')
    repo_pkg.append(repo_entry)
    ext_list.append(ext_entry.replace('#babelfishpg_common #babelfishpg_tsql #babelfishpg_tds #babelfishpg_money', '#wiltondb'))

    return repo_pkg, ext_list



def gen_param(ver, distro, indent=0, header=True):
    distro = distro.lower()
    head_pad = '  ' * indent
    text_pad = '  ' * (indent + 1) + '- '

    pkg_list, ext_list = gen_ext_list(ver, distro)
    common_packages = []
    if distro in ('rpm', 'el7', 'el8', 'el9'):
        common_packages = []
        pgsql_kernel = '%-159s # PostgreSQL %s' % ( 'postgresql%s*'%ver, ver)
    elif distro in ('deb', 'u20', 'u22', 'u24', 'd12', 'd11'):
        common_packages = []
        pgsql_kernel = '%-150s # PostgreSQL %s' % ('postgresql-%s postgresql-client-%s postgresql-server-dev-%s postgresql-plpython3-%s postgresql-plperl-%s postgresql-pltcl-%s' % (ver,ver,ver,ver,ver,ver) , ver)
    else:
        raise("invalid distro")
    pgsql_packages = [pgsql_kernel] + pkg_list
    if header:
        all_pkgs =  common_packages + pgsql_packages
    else:
        all_pkgs =  pgsql_packages
    pkg_str = '\n'.join([text_pad + i for i in all_pkgs])
    ext_str = '\n'.join([text_pad + i for i in ext_list])
    pkg_str = pkg_str.replace('-lower-quantile ', '-lower-quantile\n' + text_pad).replace(' pg_idkit_','\n' + text_pad + 'pg_idkit_')
    ext_str = ext_str.replace('lower_quantile ', 'lower_quantile\n' + text_pad)
    if header:
        return head_pad + 'repo_packages:\n%s' % pkg_str , head_pad + 'pg_extensions:\n%s' % ext_str
    else:
        return pkg_str , ext_str




def generate_all_list():
    ext_table = tabulate(
        Columns(["cat", "id", "ext3", "ver", "pkg", "lic", "rpmrepo", "debrepo", "link", "bin", "load", "dylib", "ddl", "en_desc"]),
        lambda row: True
    )
    ver_sections = []
    for ver in PG_VERS:
        buf = ["""--------\n\n## PostgreSQL %s\n\n""" % ver ]
        for distro in DISTROS:
            repo_str, ext_str = gen_param(ver, distro, -1, False)
            buf.append("""### %s OS (%s)\n\n```yaml\n%s\n```\n""" % ( DISTRO_FULLNAME.get(distro), distro, ext_str))
        ver_sections.append('\n'.join(buf))

    LIST_TEMPLATE = open(os.path.join(STUB_PATH, 'list.md')).read()
    f = openw('list.md')
    f.write(LIST_TEMPLATE % (
        STAT["stat"]["all"],STAT["stat"]["rpm"],STAT["stat"]["deb"],
        STAT["stat"]["contrib"],STAT["stat"]["non-contrib"],
        tabulate_stats(['rpm_ext', 'deb_ext']),
        ext_table,
        '\n'.join(ver_sections)
    ))
    f.close()


def generate_rpm_list():
    rpm_table = tabulate(
        Columns([ "cat", "pkg","rpmver", "lic", "rpmrepo", "rpmpkg", "r17", "r16", "r15", "r14", "r13", "r12", "en_desc"]),
        lambda row: row['has_rpm'] and row['repo'] != 'CONTRIB' and row['lead']
    )
    ver_sections = []
    for ver in PG_VERS:
        buf = ["""--------\n\n## PostgreSQL %s\n\n""" % ver ]
        for distro in ('el8', 'el9'):
            repo_str, ext_str = gen_param(ver, distro, -1, False)
            buf.append("""### %s OS (%s)\n\n```yaml\n%s\n```\n""" % ( DISTRO_FULLNAME.get(distro), distro, repo_str))
        ver_sections.append('\n'.join(buf))

    RPM_TEMPLATE = open(os.path.join(STUB_PATH, 'rpm.md')).read()
    f = openw('rpm.md')
    f.write(RPM_TEMPLATE % (
        STAT["rpm_ext"]["all"],STAT["rpm_ext"]["miss"],STAT["rpm_ext"]["miss"],
        STAT["stat"]["contrib"],STAT["rpm_ext"]["pgdg"], STAT["rpm_ext"]["pigsty"],
        STAT["rpm_ext"]["pg16"], STAT["rpm_ext"]["pg17"],
        tabulate_stats(['rpm_ext', 'rpm_pkg']),
        rpm_table,
        '\n'.join(ver_sections),
        ''
    ))
    f.close()


def generate_deb_list():
    deb_table = tabulate(
        Columns(["cat", "pkg", "debver", "lic", "debrepo", "debpkg", "d17", "d16", "d15", "d14", "d13", "d12", "en_desc"]),
        lambda row: row['has_deb'] and row['repo'] != 'CONTRIB' and row['lead']
    )
    ver_sections = []
    for ver in PG_VERS:
        buf = ["""--------\n\n## PostgreSQL %s\n\n""" % ver ]
        for distro in ('d12', 'u22', 'u24'):
            repo_str, ext_str = gen_param(ver, distro, -1, False)
            buf.append("""### %s OS (%s)\n\n```yaml\n%s\n```\n""" % ( DISTRO_FULLNAME.get(distro), distro, repo_str))
        ver_sections.append('\n'.join(buf))
    DEB_TEMPLATE = open(os.path.join(STUB_PATH, 'deb.md')).read()
    f = openw('deb.md')
    f.write(DEB_TEMPLATE % (
        STAT["deb_ext"]["all"],STAT["deb_ext"]["miss"],STAT["deb_ext"]["miss"],
        STAT["stat"]["contrib"],STAT["deb_ext"]["pgdg"], STAT["deb_ext"]["pigsty"],
        STAT["deb_ext"]["pg16"], STAT["deb_ext"]["pg17"],
        tabulate_stats(['deb_ext', 'deb_pkg']),
        deb_table,
        '\n'.join(ver_sections),
        ''
    ))
    f.close()


def generate_contrib_list():
    contrib_table = tabulate(
        Columns(["cat", "id", "ext2", "ver", "pkg", "r17", "r16", "r15", "r14", "r13", "r12","bin", "load", "dylib", "ddl","trust", "reloc", "en_desc"]),
        lambda row: row['repo'] == 'CONTRIB'
    )
    CONTRIB_TEMPLATE = open(os.path.join(STUB_PATH, 'contrib.md')).read()
    f = openw('contrib.md')
    f.write(CONTRIB_TEMPLATE % (STAT["stat"]["contrib"], contrib_table))
    f.close()



def generate_category():
    for cate in CATE:
        extensions = [row for row in DATA if row['category'] == cate]
        cate_dir = os.path.join(DOCS_PATH, cate.lower())
        cate_index = os.path.join(DOCS_PATH, cate.lower() + '.md')
        #if not os.path.exists(cate_dir): os.mkdir(cate_dir)

        exts = [ext for ext in DATA if ext['category'] == cate ]
        ext_links = [ getcol("ext4", ext) for ext in exts ]

        ext_table = tabulate(
            Columns(["id", "ext3", "ver", "pkg", "lic", "rpmrepo", "debrepo", "link", "bin", "load", "dylib", "ddl", "en_desc"]),
            lambda row: row['category'] == cate
        )
        rpm_table = tabulate(
            Columns(["pkg", "rpmver", "lic", "rpmrepo", "rpmpkg3", "r17", "r16", "r15", "r14", "r13", "r12", "en_desc"]),
            lambda row: row['has_rpm'] and row['lead'] and row['category'] == cate
        )
        deb_table = tabulate(
            Columns(["pkg", "debver", "lic", "debrepo", "debpkg3", "d17", "d16", "d15", "d14", "d13", "d12", "en_desc"]),
            lambda row: row['has_deb'] and row['lead'] and row['category'] == cate
        )

        ext_list, rpm_list, deb_list  = [], [], []
        for distro in DISTROS:
            ext_e, rpm_e, deb_e  = [], [], []
            for ver in PG_VERS:
                pkg_str, ext_str = gen_cate_ext_list(ver, distro, cate)
                ext_e.append('pg%s: %s'% (ver, ' '.join(ext_str)))
                if distro in DEB_OS:
                    deb_e.append('pg%s: %s'% (ver, ' '.join(pkg_str)))
                elif distro in RPM_OS:
                    rpm_e.append('pg%s: %s'% (ver, ' '.join(pkg_str)))

            ext_list.append('\n### %s (%s)\n\n```yaml\n%s\n```\n' % ( DISTRO_FULLNAME.get(distro,distro), distro, '\n'.join(ext_e)))
            if distro in DEB_OS:
                deb_list.append('\n### %s (%s)\n\n```yaml\n%s\n```\n' % ( DISTRO_FULLNAME.get(distro,distro), distro, '\n'.join(deb_e)))
            elif distro in RPM_OS:
                rpm_list.append('\n### %s (%s)\n\n```yaml\n%s\n```\n' % ( DISTRO_FULLNAME.get(distro,distro), distro, '\n'.join(rpm_e)))

        CATEGORY_INDEX_TEMPLATE = open(os.path.join(STUB_PATH, 'category_index.md')).read()
        with open(cate_index, 'w') as f:
            f.write(CATEGORY_INDEX_TEMPLATE%(
                cate,CATES.get(cate,''),
                len(extensions), ' '.join(ext_links),
                ext_table, '\n'.join(ext_list),
                rpm_table, '\n'.join(rpm_list),
                deb_table, '\n'.join(deb_list),
                ''
            )
        )





def generate_extension():
    for ext in DATA:
        name, alias, category = ext['name'], ext['alias'], ext['category']
        ext_path = os.path.join(DOCS_PATH, name + '.md')
        stub_path = os.path.join(STUB_PATH, name + '.md')

        header = """# %s\n\n\n> %s: %s\n>\n> %s\n\n\n""" % (ext['alias'], getcol('pkg2', ext ) ,ext['en_desc'],  ext['url'])
        # part 1: extension table
        ext_table = tabulate(
            Columns(["ext", "ver", "lic", "rpmrepo", "debrepo", "lan"]),
            lambda row: row['name'] == name
        )
        exts = [ext for ext in DATA if ext['category'] == category ]
        ext_links = [ getcol("ext4", ext) for ext in exts ]
        ext_table2 = tabulate(
            Columns(["bin", "load", "dylib", "ddl", "trust", "reloc"]),
            lambda row: row['name'] == name
        )
        ext_table3 = tabulate(Columns([ "pkg3", "tag", "schema", "req", "reqd" ]),lambda row: row['name'] == name)
        avail_table = avail_matrix(name)
        ext_table = ext_table + '\n\n\n' + ext_table2 + '\n\n\n' + ext_table3 + '\n\n\n' + avail_table

        tags,comment,schemas,config_ini,create_ddl,requires = '','','','','',''
        if ext['need_load']: config_ini = """\n```bash\nshared_preload_libraries = '%s'; # add this extension to postgresql.conf\n```\n""" % name
        if ext['need_ddl']: create_ddl = """\n```sql\nCREATE EXTENSION %s%s;\n```\n""" % ('"' + name + '"' if '-' in name else name , ' CASCADE' if ext['requires'] else '')
        if ext['comment']: comment = '> **Comment**: ' + ext['comment']
        ext_additional = config_ini + '\n\n' + create_ddl + comment #, requires + '\n' + schemas + '\n' + tags + '\n' + config_ini + '\n' + create_ddl + comment

        # part 2: package table
        rpm_table = tabulate(
            Columns(["distro", "rpmver", "lic", "rpmrepo2", "rpmpkg2", "r17", "r16", "r15", "r14", "r13", "r12", "rpmdep"]),
            lambda row: row['has_rpm'] and row['lead'] and row['alias'] == alias
        )
        deb_table = tabulate(
            Columns(["distro", "debver", "lic", "debrepo2", "debpkg2", "r17", "r16", "r15", "r14", "r13", "r12", "debdep"]),
            lambda row: row['has_deb'] and row['lead'] and row['alias'] == alias
        )
        rpm_table = rpm_table.replace('Distro-'+name, '[RPM](/rpm)')
        deb_table = '\n'.join(deb_table.replace('Distro-'+name, '[DEB](/deb)').split('\n')[2:])
        pkg_table = rpm_table + deb_table


        # install info
        pigsty_install_preface = '\nInstall `%s` via [Pigsty](https://pigsty.io/docs/pgext/usage/install/) playbook:\n\n' % getcol("alias", ext)
        if name not in ['postgis', 'citus', 'pgaudit']:
            pigsty_install_cmd = """```bash\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["%s"]}'\n```\n\n""" % alias
        else:
            if name == 'postgis': pigsty_install_cmd = """```bash\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["postgis"]}'   # postgis35, common case\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["postgis34"]}' # pg12 @ el8/9 \n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["postgis33"]}' # el7\n```\n\n"""
            if name == 'pgaudit': pigsty_install_cmd = """```bash\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["pgaudit"]}'   # common case\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["pgaudit17"]}' # pg15 @ el\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["pgaudit16"]}' # pg14 @ el\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["pgaudit15"]}' # pg13 @ el\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["pgaudit14"]}' # pg12 @ el\n```\n\n"""
            if name == 'citus': pigsty_install_cmd = """```bash\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["citus"]}'     # common case\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["citus11"]}'   # pg13 @ deb\n./pgsql.yml -t pg_extension -e '{"pg_extensions": ["citus10"]}'   # pg12 @ deb\n```\n\n"""

        yum_install_preface = '\nInstall `%s` [RPM](/rpm) from the %s **YUM** repo:\n\n' % (getcol("alias", ext), getcol("rpmrepo", ext))
        if '$v' not in ext['rpm_pkg']:
            yum_install_tmpl = """```bash\ndnf install %s;\n```\n\n""" % ext['rpm_pkg']
        else:
            pkgs = [ judge_ext(ver, 'el9', ext)[0] for ver in ext['rpm_pg'] ]
            yum_install_tmpl = """```bash\n%s\n```\n\n""" % '\n'.join([ 'dnf install ' + pkg + ';' for pkg in pkgs ])

        apt_install_preface = '\nInstall `%s` [DEB](/deb) from the %s **APT** repo:\n\n' % (getcol("alias", ext), getcol("rpmrepo", ext))
        if '$v' not in ext['rpm_pkg']:
            apt_install_tmpl = """```bash\napt install %s;\n```\n\n""" % ext['deb_pkg']
        else:
            pkgs = [ judge_ext(ver, 'u22', ext)[0] for ver in ext['deb_pg'] ]
            apt_install_tmpl = """```bash\n%s\n```\n\n""" % '\n'.join([ 'apt install ' + pkg + ';' for pkg in pkgs ])
        install_tmpl = pigsty_install_preface + pigsty_install_cmd
        if ext['has_rpm']: install_tmpl = install_tmpl + yum_install_preface + yum_install_tmpl
        if ext['has_deb']: install_tmpl = install_tmpl + apt_install_preface + apt_install_tmpl
        package_distro_matrix = avail_matrix(name,'pkg')
        install_tmpl += ('\n\n\n' + package_distro_matrix)

        stub_content = ''
        if os.path.exists(stub_path):
            stub_content = open(stub_path, 'r').read()

        EXTENSION_TEMPLATE = open(os.path.join(STUB_PATH, 'extension_page.md')).read()
        content = EXTENSION_TEMPLATE % (
            header, getcol("cat", ext), ', '.join(ext_links),
            ext_table, ext_additional,
            pkg_table, install_tmpl,
            stub_content
        )
        with open(ext_path, 'w') as f:
            f.write(content)



def cate_index_tabulate():
    cates = []
    for cate in CATE:
        cat = '[**%s**](/%s)' % (cate, cate.lower())
        exts = []
        for ext in DATA:
            if ext['category'] == cate:
                exts.append(getcol("ext4", ext))
        cates.append(cat + ': ' + ' '.join(exts))
    return '\n'.join(cates)


def cate_index_tabulate2():
    header = '| Category | Extensions |'
    align =  '|:--------:|------------|'
    cates = [header, align]
    for cate in CATE:
        cat = '[**%s**](/%s)' % (cate, cate.lower())
        exts = []
        for ext in DATA:
            if ext['category'] == cate:
                exts.append(getcol("ext4", ext))
        cates.append('|  %s  |  %s  |' % (cat,' '.join(exts)))
    return '\n'.join(cates)




def generate_categories():
    data = []
    for cate in CATE_LIST:
        extensions = [row for row in DATA if row['category'] == cate]
        exts = [ext for ext in DATA if ext['category'] == cate ]
        ext_links = [ getcol("ext4", ext) for ext in exts ]
        ext_table = tabulate(
            Columns(["id", "ext3", "ver", "pkg", "lic", "rpmrepo", "debrepo", "link", "en_desc", "comment"]),
            lambda row: row['category'] == cate
        )
        cate_info = """\n--------\n## [**%s**](/%s)\n> %s\n\n%s""" %(
            cate, cate.lower(), CATES.get(cate,'') + ' (%d extensions)' % len(exts), ext_table
        )

        data.append(cate_info)


    CATEGORIES_TEMPLATE = open(os.path.join(STUB_PATH, 'categories.md')).read()
    f = openw('categories.md')
    f.write(CATEGORIES_TEMPLATE%(
        len(CATE_LIST),'\n'.join(data)
    ))



def generate_readme():
    with open(os.path.join(STUB_PATH, 'README.md'), 'r') as src:
        readme_tmpl = src.read()
    ext_num = len(DATA)
    f = openw('README.md')
    f.write(readme_tmpl% (ext_num,ext_num,ext_num,
                          tabulate_stats(['rpm_ext', 'deb_ext']),
                          cate_index_tabulate()
                          ))
    f.close()



generate_readme()
generate_all_list()
generate_rpm_list()
generate_deb_list()
generate_contrib_list()
generate_categories()
generate_category()
generate_extension()
