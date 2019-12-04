// Copyright 2018 gf Author(https://github.com/gogf/gf). All Rights Reserved.
//
// This Source Code Form is subject to the terms of the MIT License.
// If a copy of the MIT was not distributed with this file,
// You can obtain one at https://github.com/gogf/gf.

package ghttp

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/gogf/gf/os/gfile"
	"github.com/gogf/gf/os/glog"
	"github.com/gogf/gf/text/gregex"
	"github.com/gogf/gf/text/gstr"
)

// 绑定控制器，控制器需要实现 gmvc.Controller 接口,
// 这种方式绑定的控制器每一次请求都会初始化一个新的控制器对象进行处理，对应不同的请求会话,
// 第三个参数methods用以指定需要注册的方法，支持多个方法名称，多个方法以英文“,”号分隔，区分大小写.
func (s *Server) BindController(pattern string, controller Controller, method ...string) {
	bindMethod := ""
	if len(method) > 0 {
		bindMethod = method[0]
	}
	s.doBindController(pattern, controller, bindMethod, nil)
}

// 绑定路由到指定的方法执行, 第三个参数method仅支持一个方法注册，不支持多个，并且区分大小写。
func (s *Server) BindControllerMethod(pattern string, controller Controller, method string) {
	s.doBindControllerMethod(pattern, controller, method, nil)
}

// 绑定控制器(RESTFul)，控制器需要实现gmvc.Controller接口
// 方法会识别HTTP方法，并做REST绑定处理，例如：Post方法会绑定到HTTP POST的方法请求处理，Delete方法会绑定到HTTP DELETE的方法请求处理
// 因此只会绑定HTTP Method对应的方法，其他方法不会自动注册绑定
// 这种方式绑定的控制器每一次请求都会初始化一个新的控制器对象进行处理，对应不同的请求会话
func (s *Server) BindControllerRest(pattern string, controller Controller) {
	s.doBindControllerRest(pattern, controller, nil)
}

func (s *Server) doBindController(pattern string, controller Controller, method string, middleware []HandlerFunc) {
	// Convert input method to map for convenience and high performance searching.
	var methodMap map[string]bool
	if len(method) > 0 {
		methodMap = make(map[string]bool)
		for _, v := range strings.Split(method, ",") {
			methodMap[strings.TrimSpace(v)] = true
		}
	}
	// 当pattern中的method为all时，去掉该method，以便于后续方法判断
	domain, method, path, err := s.parsePattern(pattern)
	if err != nil {
		glog.Error(err)
		return
	}
	if strings.EqualFold(method, gDEFAULT_METHOD) {
		pattern = s.serveHandlerKey("", path, domain)
	}
	// 遍历控制器，获取方法列表，并构造成uri
	m := make(handlerMap)
	v := reflect.ValueOf(controller)
	t := v.Type()
	sname := t.Elem().Name()
	pkgPath := t.Elem().PkgPath()
	pkgName := gfile.Basename(pkgPath)
	for i := 0; i < v.NumMethod(); i++ {
		mname := t.Method(i).Name
		if methodMap != nil && !methodMap[mname] {
			continue
		}
		if mname == "Init" || mname == "Shut" || mname == "Exit" {
			continue
		}
		ctlName := gstr.Replace(t.String(), fmt.Sprintf(`%s.`, pkgName), "")
		if ctlName[0] == '*' {
			ctlName = fmt.Sprintf(`(%s)`, ctlName)
		}
		if _, ok := v.Method(i).Interface().(func()); !ok {
			if len(methodMap) > 0 {
				// 指定的方法名称注册，那么需要使用错误提示
				glog.Errorf(`invalid route method: %s.%s.%s defined as "%s", but "func()" is required for controller registry`,
					pkgPath, ctlName, mname, v.Method(i).Type().String())
			} else {
				// 否则只是Debug提示
				glog.Debugf(`ignore route method: %s.%s.%s defined as "%s", no match "func()"`,
					pkgPath, ctlName, mname, v.Method(i).Type().String())
			}
			continue
		}
		key := s.mergeBuildInNameToPattern(pattern, sname, mname, true)
		m[key] = &handlerItem{
			itemName: fmt.Sprintf(`%s.%s.%s`, pkgPath, ctlName, mname),
			itemType: gHANDLER_TYPE_CONTROLLER,
			ctrlInfo: &handlerController{
				name:    mname,
				reflect: v.Elem().Type(),
			},
			middleware: middleware,
		}
		// 如果方法中带有Index方法，那么额外自动增加一个路由规则匹配主URI，
		// 例如: pattern为/user, 那么会同时注册/user及/user/index，
		// 这里处理新增/user路由绑定。
		// 注意，当pattern带有内置变量时，不会自动加该路由。
		if strings.EqualFold(mname, "Index") && !gregex.IsMatchString(`\{\.\w+\}`, pattern) {
			p := gstr.PosRI(key, "/index")
			k := key[0:p] + key[p+6:]
			if len(k) == 0 || k[0] == '@' {
				k = "/" + k
			}
			m[k] = &handlerItem{
				itemName: fmt.Sprintf(`%s.%s.%s`, pkgPath, ctlName, mname),
				itemType: gHANDLER_TYPE_CONTROLLER,
				ctrlInfo: &handlerController{
					name:    mname,
					reflect: v.Elem().Type(),
				},
				middleware: middleware,
			}
		}
	}
	s.bindHandlerByMap(m)
}

func (s *Server) doBindControllerMethod(pattern string, controller Controller, method string, middleware []HandlerFunc) {
	m := make(handlerMap)
	v := reflect.ValueOf(controller)
	t := v.Type()
	sname := t.Elem().Name()
	mname := strings.TrimSpace(method)
	fval := v.MethodByName(mname)
	if !fval.IsValid() {
		glog.Error("invalid method name:" + mname)
		return
	}
	pkgPath := t.Elem().PkgPath()
	pkgName := gfile.Basename(pkgPath)
	ctlName := gstr.Replace(t.String(), fmt.Sprintf(`%s.`, pkgName), "")
	if ctlName[0] == '*' {
		ctlName = fmt.Sprintf(`(%s)`, ctlName)
	}
	if _, ok := fval.Interface().(func()); !ok {
		glog.Errorf(`invalid route method: %s.%s.%s defined as "%s", but "func()" is required for controller registry`,
			pkgPath, ctlName, mname, fval.Type().String())
		return
	}
	key := s.mergeBuildInNameToPattern(pattern, sname, mname, false)
	m[key] = &handlerItem{
		itemName: fmt.Sprintf(`%s.%s.%s`, pkgPath, ctlName, mname),
		itemType: gHANDLER_TYPE_CONTROLLER,
		ctrlInfo: &handlerController{
			name:    mname,
			reflect: v.Elem().Type(),
		},
		middleware: middleware,
	}
	s.bindHandlerByMap(m)
}

func (s *Server) doBindControllerRest(pattern string, controller Controller, middleware []HandlerFunc) {
	// 遍历控制器，获取方法列表，并构造成uri
	m := make(handlerMap)
	v := reflect.ValueOf(controller)
	t := v.Type()
	sname := t.Elem().Name()
	pkgPath := t.Elem().PkgPath()
	// 如果存在与HttpMethod对应名字的方法，那么绑定这些方法
	for i := 0; i < v.NumMethod(); i++ {
		mname := t.Method(i).Name
		method := strings.ToUpper(mname)
		if _, ok := methodsMap[method]; !ok {
			continue
		}
		pkgName := gfile.Basename(pkgPath)
		ctlName := gstr.Replace(t.String(), fmt.Sprintf(`%s.`, pkgName), "")
		if ctlName[0] == '*' {
			ctlName = fmt.Sprintf(`(%s)`, ctlName)
		}
		if _, ok := v.Method(i).Interface().(func()); !ok {
			glog.Errorf(`invalid route method: %s.%s.%s defined as "%s", but "func()" is required for controller registry`,
				pkgPath, ctlName, mname, v.Method(i).Type().String())
			return
		}
		key := s.mergeBuildInNameToPattern(mname+":"+pattern, sname, mname, false)
		m[key] = &handlerItem{
			itemName: fmt.Sprintf(`%s.%s.%s`, pkgPath, ctlName, mname),
			itemType: gHANDLER_TYPE_CONTROLLER,
			ctrlInfo: &handlerController{
				name:    mname,
				reflect: v.Elem().Type(),
			},
			middleware: middleware,
		}
	}
	s.bindHandlerByMap(m)
}
