package script

import (
	"errors"
	"fmt"
	"github.com/aarzilli/golua/lua"
	"github.com/stevedonovan/luar"
	anko_core "github.com/mattn/anko/builtins"
	anko_encoding "github.com/mattn/anko/builtins/encoding"
	anko_flag "github.com/mattn/anko/builtins/flag"
	anko_io "github.com/mattn/anko/builtins/io"
	anko_math "github.com/mattn/anko/builtins/math"
	anko_net "github.com/mattn/anko/builtins/net"
	anko_os "github.com/mattn/anko/builtins/os"
	anko_path "github.com/mattn/anko/builtins/path"
	anko_regexp "github.com/mattn/anko/builtins/regexp"
	anko_sort "github.com/mattn/anko/builtins/sort"
	anko_strings "github.com/mattn/anko/builtins/strings"
	anko_term "github.com/mattn/anko/builtins/term"
	anko "github.com/mattn/anko/vm"
	"github.com/robertkrimen/otto"
	"github.com/tyler-sommer/squircy2/squircy/event"
	"github.com/tyler-sommer/squircy2/squircy/data"
	glispext "github.com/zhemao/glisp/extensions"
	glisp "github.com/zhemao/glisp/interpreter"
	"reflect"
	"strconv"
	"crypto/sha1"
)



func newJavascriptVm(m *ScriptManager) *otto.Otto {
	getFnName := func(fn otto.Value) (name string) {
		if fn.IsFunction() {
			name = fmt.Sprintf("__Handler%x", sha1.Sum([]byte(fn.String())))
		} else {
			name = fn.String()
		}

		return
	}

	jsVm := otto.New()
	jsVm.Set("Http", &m.httpHelper)
	jsVm.Set("Config", &m.configHelper)
	jsVm.Set("Data", &m.dataHelper)
	jsVm.Set("Irc", &m.ircHelper)
	jsVm.Set("bind", func(call otto.FunctionCall) otto.Value {
		eventType := call.Argument(0).String()
		fn := call.Argument(1)
		fnName := getFnName(fn)
		if fn.IsFunction() {
			m.jsDriver.vm.Set(fnName, func(call otto.FunctionCall) otto.Value {
				fn.Call(call.This, call.ArgumentList)
				return otto.UndefinedValue()
			})
		}
		m.scriptHelper.Bind(Javascript, event.EventType(eventType), fnName)
		val, _ := otto.ToValue(fnName)
		return val
	})
	jsVm.Set("unbind", func(call otto.FunctionCall) otto.Value {
		eventType := call.Argument(0).String()
		fnName := getFnName(call.Argument(1))
		m.scriptHelper.Unbind(Javascript, event.EventType(eventType), fnName)
		return otto.UndefinedValue()
	})
	jsVm.Set("trigger", func(call otto.FunctionCall) otto.Value {
		eventType := call.Argument(0).String()
		data, _ := call.Argument(1).Export()
		if data == nil {
			data = make(map[string]interface{}, 0)
		}
		m.scriptHelper.Trigger(event.EventType(eventType), data.(map[string]interface{}))
		return otto.UndefinedValue()
	})
	jsVm.Set("use", func(call otto.FunctionCall) otto.Value {
		coll := call.Argument(0).String()

		// Todo: get the Database properly
		db := data.NewGenericRepository(m.repo.database, coll)
		obj, _ := jsVm.Object("({})")
		obj.Set("Save", func(call otto.FunctionCall) otto.Value {
			exp, _ := call.Argument(0).Export()
			var model data.GenericModel
			switch t := exp.(type) {
			case data.GenericModel:
				model = t

			case map[string]interface{}:
				model = data.GenericModel(t)
			}
			switch t := model["ID"].(type) {
			case int64:
				model["ID"] = int(t)

			case int:
				model["ID"] = t
			}
			db.Save(model)

			id, _ := jsVm.ToValue(model["ID"])

			return id
		})
		obj.Set("Fetch", func(call otto.FunctionCall) otto.Value {
			i, _ := call.Argument(0).ToInteger()
			val := db.Fetch(int(i))
			v, err := jsVm.ToValue(val)

			if err != nil {
				panic(err)
			}

			return v
		})
		obj.Set("FetchAll", func(call otto.FunctionCall) otto.Value {
			vals := db.FetchAll()
			v, err := jsVm.ToValue(vals)

			if err != nil {
				panic(err)
			}

			return v
		})
		obj.Set("Index", func(call otto.FunctionCall) otto.Value {
			exp, _ := call.Argument(0).Export()
			cols := make([]string, 0)
			for _, val := range exp.([]interface{}) {
				cols = append(cols, val.(string))
			}
			db.Index(cols)

			return otto.UndefinedValue()
		})
		obj.Set("Query", func(call otto.FunctionCall) otto.Value {
			qry, _ := call.Argument(0).Export()
			vals := db.Query(qry)
			v, err := jsVm.ToValue(vals)

			if err != nil {
				panic(err)
			}

			return v
		})

		return obj.Value()
	})


	return jsVm
}

func newLuaVm(m *ScriptManager) *lua.State {
	luaVm := luar.Init()
	luaVm.Register("typename", func(vm *lua.State) int {
		o := vm.Typename(int(vm.Type(1)))
		vm.PushString(o)
		return 1
	})
	luar.Register(luaVm, "", luar.Map{
		"http": m.httpHelper,
		"config": m.configHelper,
		"data": m.dataHelper,
		"irc": m.ircHelper,
	})
	luaVm.Register("bind", func(vm *lua.State) int {
		eventType := vm.ToString(1)
		fnName := vm.ToString(2)
		m.scriptHelper.Bind(Lua, event.EventType(eventType), fnName)
		return 0
	})
	luaVm.Register("unbind", func(vm *lua.State) int {
		eventType := vm.ToString(1)
		fnName := vm.ToString(2)
		m.scriptHelper.Unbind(Lua, event.EventType(eventType), fnName)
		return 0
	})
	luaVm.Register("trigger", func(vm *lua.State) int {
		eventType := vm.ToString(1)
		data := luar.LuaToGo(vm, nil, 2)
		if data == nil {
			data = make(map[string]interface{}, 0)
		}
		m.scriptHelper.Trigger(event.EventType(eventType), data.(map[string]interface{}))
		return 0
	})

	return luaVm
}

func newLispVm(m *ScriptManager) *glisp.Glisp {
	lispVm := glisp.NewGlisp()
	lispVm.ImportEval()
	glispext.ImportRandom(lispVm)
	glispext.ImportTime(lispVm)
	glispext.ImportChannels(lispVm)
	glispext.ImportCoroutines(lispVm)

	lispVm.AddFunction("setex", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 2 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		key := sexpToString(args[0])
		val := exportSexp(args[1])
		m.dataHelper.Set(key, val)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("getex", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		key := sexpToString(args[0])
		val := m.dataHelper.Get(key).(string)

		return glisp.SexpStr(val), nil
	})
	lispVm.AddFunction("joinchan", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		channel := sexpToString(args[0])
		m.ircHelper.Join(channel)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("partchan", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		channel := sexpToString(args[0])
		m.ircHelper.Part(channel)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("privmsg", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 2 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		target := sexpToString(args[0])
		message := sexpToString(args[1])
		m.ircHelper.Privmsg(target, message)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("currentnick", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		res := m.ircHelper.CurrentNick()

		return glisp.SexpStr(res), nil
	})
	lispVm.AddFunction("nick", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		newNick := sexpToString(args[0])
		m.ircHelper.Nick(newNick)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("httpget", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		url := sexpToString(args[0])
		resp := m.httpHelper.Get(url)

		return glisp.SexpStr(resp), nil
	})
	lispVm.AddFunction("bind", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 2 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		eventType := sexpToString(args[0])
		fnName := sexpToString(args[1])
		m.scriptHelper.Bind(Lisp, event.EventType(eventType), fnName)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("unbind", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 2 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		eventType := sexpToString(args[0])
		fnName := sexpToString(args[1])
		m.scriptHelper.Unbind(Lisp, event.EventType(eventType), fnName)

		return glisp.SexpNull, nil
	})
	lispVm.AddFunction("parse-integer", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		switch t := args[0].(type) {
		case glisp.SexpStr:
			val, err := strconv.ParseInt(string(t), 0, 64)
			if err != nil {
				return glisp.SexpNull, err
			}
			return glisp.SexpInt(int(val)), nil

		default:
			return glisp.SexpNull, errors.New(fmt.Sprintf("cannot convert %v to int", t))
		}
	})
	lispVm.AddFunction("write-to-string", func(vm *glisp.Glisp, name string, args []glisp.Sexp) (glisp.Sexp, error) {
		if len(args) != 1 {
			return glisp.SexpNull, errors.New("incorrect number of arguments")
		}

		switch t := args[0].(type) {
		case glisp.SexpStr:
			return t, nil

		case glisp.SexpInt:
			return glisp.SexpStr(strconv.Itoa(int(t))), nil

		default:
			return glisp.SexpStr(fmt.Sprintf("%v", t)), nil
		}
	})

	return lispVm
}

func newAnkoVm(m *ScriptManager) *anko.Env {
	ankoVm := anko.NewEnv()
	anko_core.Import(ankoVm)
	anko_flag.Import(ankoVm)
	anko_net.Import(ankoVm)
	anko_encoding.Import(ankoVm)
	anko_os.Import(ankoVm)
	anko_io.Import(ankoVm)
	anko_math.Import(ankoVm)
	anko_path.Import(ankoVm)
	anko_regexp.Import(ankoVm)
	anko_sort.Import(ankoVm)
	anko_strings.Import(ankoVm)
	anko_term.Import(ankoVm)

	mod := ankoVm.NewModule("data")
	mod.Define("Get", reflect.ValueOf(m.dataHelper.Get))
	mod.Define("Set", reflect.ValueOf(m.dataHelper.Set))

	mod = ankoVm.NewModule("irc")
	mod.Define("Join", reflect.ValueOf(func(channel string) {
		m.ircHelper.Join(channel)
	}))
	mod.Define("Part", reflect.ValueOf(func(channel string) {
		m.ircHelper.Part(channel)
	}))
	mod.Define("Privmsg", reflect.ValueOf(func(target, message string) {
		m.ircHelper.Privmsg(target, message)
	}))
	mod.Define("CurrentNick", reflect.ValueOf(func() string {
		return m.ircHelper.CurrentNick()
	}))
	mod.Define("Nick", reflect.ValueOf(func(newNick string) {
		m.ircHelper.Nick(newNick)
	}))

	mod = ankoVm.NewModule("strconv")
	mod.Define("ParseInt", reflect.ValueOf(func(s string) int {
		i, _ := strconv.ParseInt(s, 0, 64)
		return int(i)
	}))

	ankoVm.Define("bind", reflect.ValueOf(func(eventType, fnName string) {
		m.scriptHelper.Bind(Anko, event.EventType(eventType), fnName)
	}))

	ankoVm.Define("unbind", reflect.ValueOf(func(eventType, fnName string) {
		m.scriptHelper.Unbind(Anko, event.EventType(eventType), fnName)
	}))

	return ankoVm
}
